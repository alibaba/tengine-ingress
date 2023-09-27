/*
Copyright 2022 The Alibaba Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/ingress-nginx/internal/ingress"
	ngx_config "k8s.io/ingress-nginx/internal/ingress/controller/config"
	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/internal/lock"
	"k8s.io/ingress-nginx/internal/route"
	"k8s.io/ingress-nginx/internal/shm"
	"k8s.io/klog"
)

type CfgType uint32

const (
	// Service config in hot reload
	ServiceCfg CfgType = iota + 1
)

const (
	// Hot reload init
	HotReloadInit = 99999999

	// Hot reload is successful
	HotReloadSuccess = 0
)

const (
	// #1 Number of bytes (network big endian byte order) to store status of hot reload in shm
	ShmStatusBytes = 4

	// #2 Number of bytes (network big endian byte order) to store timestamp in shm
	ShmTimestampBytes = 8

	// #3 Number of bytes (network big endian byte order) to store cofig type in shm
	ShmCfgTypeBytes = 4

	// #4 Number of bytes (string) to store m5d of config in shm
	ShmCfgMd5Bytes = 32

	// #5 Number of bytes (network big endian byte order) to store length of config in shm
	ShmCfgLenBytes = 4

	// Number of bytes of above items in shm
	ShmCtlBytesSum = 52

	// #6 config (protobuf) in shm
)

const (
	// Min value of mod divisor
	MinModDivisor = 2
	// Max value of mod divisor
	MaxModDivisor = 100
)

const (
	// Service cofig shm
	ShmServiceCfg = "ShmServiceCfg"
)

const (
	// Metadata namespace
	MetaNamespace = "namespace"

	// Metadata ingress name
	MetaIngressName = "ingress_name"

	// Metadata service name
	MetaServiceName = "service_name"

	// Metadata service port
	MetaServicePort = "service_port"

	// Metadata location path
	MetaLocationPath = "location_path"

	// Metadata disable robots
	MetaDisableRobots = "disable-robots"

	// Matadata CORS enable-cors
	MetaEnableCors = "enable-cors"

	// Matadata CORS cors-allow-origin
	MetaCorsAllowOrigin = "cors-allow-origin"

	// Matadata CORS cors-max-age
	MetaCorsMaxAge = "cors-max-age"

	// Matadata CORS cors-allow-credentials
	MetaCorsAllowCredentials = "cors-allow-credentials"

	// Matadata CORS cors-allow-methods
	MetaCorsAllowMethods = "cors-allow-methods"

	// Matadata CORS cors-allow-headers
	MetaCorsAllowHeaders = "cors-allow-headers"
)

const (
	// Defauly server name
	DefServerName = "_"
	// Root path
	RootLocation = "/"
	// Canary
	Canary = "Canary"
	// Default value of Header and cookie
	Always = "always"
	// Separator of canary annotation
	// <header value>[||<header value>]*
	// <cookie value>[||<cookie value>]*
	// <query value>[||<query value>]*
	// <header name>:<header value>[||<header name>:<header value>]*
	CanaryDelimiter = "||"
	// URL query parameter separator
	QueryDelimiter = "&"
	// URL query value separator
	QueryValDelimiter = "="
	// Header field consists of a name followed by a colon (":") and the field value
	HeaderDelimiter = ":"
	// Nginx variable '&'
	NginxVar = "$"
	// Mod relational operator '>'
	ModOperGreater = ">"
	// Mod relational operator '>='
	ModOperGreaterEqual = ">="
	// Mod relational operator '<'
	ModOperLess = "<"
	// Mod relational operator '<='
	ModOperLessEqual = "<="
	// Mod relational operator '=='
	ModOperEqual = "=="
)

func hotReload(oldMD5 string, cfg ngx_config.Configuration, ingressCfg ingress.Configuration, init bool) (string, error) {
	var hotCfg *route.Config
	var newMD5 string
	hotCfg = createHotCfg(cfg, ingressCfg)
	cfgbuf, ret := hotCfg.Marshal()
	if ret != nil {
		klog.Errorf("Hot reload failed for marshal")
		return oldMD5, ret
	}

	newMD5 = fmt.Sprintf("%x", md5.Sum(cfgbuf))
	if newMD5 == oldMD5 {
		klog.Errorf("Skipping hot reload for the same md5 [%v]", newMD5)
		return oldMD5, nil
	}

	cfgLen := uint32(len(cfgbuf))
	now := time.Now()
	nsec := now.UnixNano()
	klog.Infof("Hot reloading cfg: {init[%v], status[%v],timestamp[%v, %v],type[%v],md5[%v],length[%v],cfg[%v]}",
		init, HotReloadInit, now, nsec, ServiceCfg, newMD5, cfgLen, hotCfg.GoString())

	shmLen := ShmCtlBytesSum + cfgLen
	shmBuf := make([]byte, shmLen)
	binary.BigEndian.PutUint32(shmBuf[0:], uint32(HotReloadInit))
	binary.BigEndian.PutUint64(shmBuf[4:], uint64(nsec))
	binary.BigEndian.PutUint32(shmBuf[12:], uint32(ServiceCfg))
	copy(shmBuf[16:], newMD5)
	binary.BigEndian.PutUint32(shmBuf[48:], cfgLen)
	copy(shmBuf[52:], cfgbuf)

	var err error
	var num int
	var mem *shm.Memory
	file := lock.AcquireFileLock(cfg.ShmServiceCfgFileLock)
	if init {
		mem, err = shm.Create(ShmServiceCfg, cfg.IngressShmSize)
	} else {
		mem, err = shm.Open(ShmServiceCfg, cfg.IngressShmSize)
	}

	if err != nil {
		klog.Errorf("Hot reload failed for init [%v] shm with err [%v]", init, err)
		goto done
	}

	num, err = mem.WriteAt(shmBuf, 0)
	lock.ReleaseFileLock(file)
	if err != nil {
		klog.Errorf("Hot reload failed for writing shm with err [%v]", err)
		goto done
	}

	if num != int(shmLen) {
		err = errors.New(fmt.Sprintf("write shm %v bytes, want %v bytes", num, shmLen))
		goto done
	}

	klog.Infof("Hot reloading with shm: {name[%v],size[%v]}", ShmServiceCfg, num)
done:
	if err != nil {
		lock.ReleaseFileLock(file)
		klog.Errorf("Close shm [%v] with the hot reload fails", err)
		return oldMD5, err
	}

	klog.Infof("Hot reloading successful")
	return newMD5, nil
}

func createHotCfg(cfg ngx_config.Configuration, ingressCfg ingress.Configuration) *route.Config {
	services := make([]*route.VirtualService, 0, len(ingressCfg.Servers))
	routers := make([]*route.Router, 0, len(ingressCfg.Servers))
	for _, server := range ingressCfg.Servers {
		if server.Hostname == DefServerName {
			continue
		}

		var router *route.Router
		router = createHostRouter(cfg, server, &services)
		routers = append(routers, router)
	}

	return &route.Config{
		Routers:  routers,
		Services: services,
	}
}

func createHostRouter(cfg ngx_config.Configuration, server *ingress.Server, services *[]*route.VirtualService) *route.Router {
	rootServiceName := server.Hostname + RootLocation
	paths := make([]*route.PathRouter, 0, len(server.Locations))

	for _, loc := range server.Locations {
		upsService := loc.Service
		pathServiceName := server.Hostname + loc.Path
		if upsService == nil {
			continue
		}

		service := createVirtualService(cfg, loc.Backend, loc, pathServiceName, server)
		path := &route.PathRouter{
			Prefix:      loc.Path,
			ServiceName: pathServiceName,
		}

		if len(loc.Canaries) > 0 {
			tags := make([]*route.TagRouter, 0, len(loc.Canaries))
			canaryUps := make([]*route.Upstream, 0, len(loc.Canaries))
			setCanaryPriority(&loc.Canaries)
			for i, canary := range loc.Canaries {
				if i == cfg.MaxCanaryIngNum {
					klog.Errorf("Loc[%v%v] exceeds the canary limit [%v], ignored",
						server.Hostname, loc.Path, cfg.MaxCanaryIngNum)
					break
				}
				canaryService := &route.VirtualService{}
				tagRouter := &route.TagRouter{}
				policy := canary.TrafficShapingPolicy
				if len(policy.Header) > 0 {
					canaryService, tagRouter = createHeaderCanary(i, cfg, server, loc, canary)
					tags = append(tags, tagRouter)
					*services = append(*services, canaryService)
				} else if len(policy.Cookie) > 0 {
					canaryService, tagRouter = createCookieCanary(i, cfg, server, loc, canary)
					tags = append(tags, tagRouter)
					*services = append(*services, canaryService)
				} else if len(policy.Query) > 0 {
					canaryService, tagRouter = createQueryCanary(i, cfg, server, loc, canary)
					tags = append(tags, tagRouter)
					*services = append(*services, canaryService)
				} else if policy.Weight > 0 {
					canaryUpstream := createWeightCanary(server, loc, canary)
					canaryUps = append(canaryUps, canaryUpstream)
				} else {
					klog.Errorf("Loc[%v%v] canary is invalid, ignored", server.Hostname, loc.Path)
					continue
				}
			}

			path.Tags = tags
			upstreamWeight(cfg, canaryUps, loc, service)
		}
		paths = append(paths, path)
		*services = append(*services, service)
	}

	hostRouter := &route.HostRouter{
		Host:        server.Hostname,
		ServiceName: rootServiceName,
		Paths:       paths,
		Type:        ingType,
	}

	router := &route.Router{
		HostRouter: hostRouter,
	}

	return router
}

func createHostAPIRouter(cfg ngx_config.Configuration, ingType route.HostType, server *ingress.Server, services *[]*route.VirtualService) *route.Router {
	rootServiceName := server.Hostname + RootLocation
	for _, loc := range server.Locations {
		if loc.Path == RootLocation {
			service := createHostAPIVirtualService(loc, rootServiceName, server)
			*services = append(*services, service)
			break
		}
	}

	hostRouter := &route.HostRouter{
		Host:        server.Hostname,
		ServiceName: rootServiceName,
		Type:        ingType,
	}

	router := &route.Router{
		HostRouter: hostRouter,
	}

	return router
}

func createVirtualService(cfg ngx_config.Configuration, target string, loc *ingress.Location, serviceName string, server *ingress.Server) *route.VirtualService {
	upstream := &route.Upstream{
		Target: target,
		Weight: cfg.DefaultCanaryWeightTotal,
	}

	timeout := &route.Timeout{
		ReadTimeout: uint32(loc.Proxy.ReadTimeout) * 1000,
	}

	service := &route.VirtualService{
		ServiceName: serviceName,
		Upstreams: []*route.Upstream{
			upstream,
		},
		TimeoutMs:  timeout,
		ForceHttps: loc.Rewrite.SSLRedirect,
		Metadata:   createMetaData(server, loc),
	}

	return service
}

func createMetaData(server *ingress.Server, loc *ingress.Location) []*route.Metadata {
	var namespace, ingress, service string
	if loc.Ingress == nil {
		namespace = ""
		ingress = ""
		service = ""
	} else {
		namespace = loc.Ingress.Namespace
		ingress = loc.Ingress.Name
		service = k8s.MetaNamespaceKey(loc.Service)
	}

	return []*route.Metadata{
		&route.Metadata{
			Key:   MetaNamespace,
			Value: namespace,
		},
		&route.Metadata{
			Key:   MetaIngressName,
			Value: ingress,
		},
		&route.Metadata{
			Key:   MetaServiceName,
			Value: service,
		},
		&route.Metadata{
			Key:   MetaServicePort,
			Value: loc.Port.StrVal,
		},
		&route.Metadata{
			Key:   MetaLocationPath,
			Value: loc.Path,
		},
		&route.Metadata{
			Key:   MetaDisableRobots,
			Value: strconv.FormatBool(loc.DisableRobots),
		},
		&route.Metadata{
			Key:   MetaEnableCors,
			Value: strconv.FormatBool(loc.CorsConfig.CorsEnabled),
		},
		&route.Metadata{
			Key:   MetaCorsAllowOrigin,
			Value: loc.CorsConfig.CorsAllowOrigin,
		},
		&route.Metadata{
			Key:   MetaCorsAllowMethods,
			Value: loc.CorsConfig.CorsAllowMethods,
		},
		&route.Metadata{
			Key:   MetaCorsAllowHeaders,
			Value: loc.CorsConfig.CorsAllowHeaders,
		},
		&route.Metadata{
			Key:   MetaCorsAllowCredentials,
			Value: strconv.FormatBool(loc.CorsConfig.CorsAllowCredentials),
		},
		&route.Metadata{
			Key:   MetaCorsMaxAge,
			Value: strconv.FormatInt(int64(loc.CorsConfig.CorsMaxAge), 10),
		},
	}
}

func getValType(str string) route.ActionValueType {
	if strings.HasPrefix(str, NginxVar) {
		return route.ActionDynamicValue
	}

	return route.ActionStaticValue
}

func getVal(valType route.ActionValueType, str string) string {
	if valType == route.ActionDynamicValue {
		return strings.TrimPrefix(str, NginxVar)
	}

	return str
}

func createAction(maxActionNum int, actionType route.ActionType, canaryDelimiter string, itemDelimiter string, policyRule string, canary *ingress.Canary, service *route.VirtualService, actions *[]*route.Action) {
	policies := strings.Split(policyRule, canaryDelimiter)
	num := 0
	for _, policy := range policies {
		if policy == "" {
			klog.V(3).Infof("Policy rule for action is empty")
			continue
		}

		klog.V(3).Infof("Policy rule for action is [%v]", policy)
		itmes := strings.Split(policy, itemDelimiter)
		if len(itmes) == 2 && len(itmes[0]) > 0 && len(itmes[1]) > 0 {
			if num == maxActionNum {
				klog.Errorf("Service [%v] with backend [%v] exceeds the action [%v] limit [%v], [%v%v%v] ignored",
					service.ServiceName, canary.Target, actionType, maxActionNum, itmes[0], itemDelimiter, itmes[1])
				break
			}

			valType := getValType(itmes[1])
			val := getVal(valType, itmes[1])
			action := &route.Action{
				ActionType: actionType,
				ValueType:  valType,
				Key:        itmes[0],
				Value:      val,
			}

			*actions = append(*actions, action)
			num++
		}
	}
}

func createActions(cfg ngx_config.Configuration, canary *ingress.Canary, service *route.VirtualService) {
	policy := canary.TrafficShapingPolicy
	exist := len(policy.ReqAddHeader) + len(policy.ReqAppendHeader) + len(policy.ReqAddQuery) + len(policy.RespAddHeader) + len(policy.RespAppendHeader)
	if exist == 0 {
		return
	}

	actions := make([]*route.Action, 0, cfg.MaxCanaryActionNum)
	if len(policy.ReqAddHeader) > 0 {
		createAction(cfg.MaxReqAddHeaderNum, route.ActionAddReqHeader, CanaryDelimiter, HeaderDelimiter, policy.ReqAddHeader, canary, service, &actions)
	}
	if len(policy.ReqAppendHeader) > 0 {
		createAction(cfg.MaxReqAppendHeaderNum, route.ActionAppendReqHeader, CanaryDelimiter, HeaderDelimiter, policy.ReqAppendHeader, canary, service, &actions)
	}
	if len(policy.ReqAddQuery) > 0 {
		createAction(cfg.MaxReqAddQueryNum, route.ActionAddParam, QueryDelimiter, QueryValDelimiter, policy.ReqAddQuery, canary, service, &actions)
	}
	if len(policy.RespAddHeader) > 0 {
		createAction(cfg.MaxRespAddHeaderNum, route.ActionAddRespHeader, CanaryDelimiter, HeaderDelimiter, policy.RespAddHeader, canary, service, &actions)
	}
	if len(policy.RespAppendHeader) > 0 {
		createAction(cfg.MaxRespAppendHeaderNum, route.ActionAppendRespHeader, CanaryDelimiter, HeaderDelimiter, policy.RespAppendHeader, canary, service, &actions)
	}

	service.Action = actions
}

func checkModDivisor(modDivisor uint64) bool {
	if modDivisor >= MinModDivisor && modDivisor <= MaxModDivisor {
		return true
	}

	return false
}

func checkModRemainder(modDivisor uint64, modRemainder uint64) bool {
	if modRemainder >= 0 && modRemainder < modDivisor {
		return true
	}

	return false
}

func getModOper(modOpr string) (bool, route.OperatorType) {
	if modOpr == ModOperEqual {
		return true, route.OperatorEqual
	} else if modOpr == ModOperGreater {
		return true, route.OperatorGreater
	} else if modOpr == ModOperGreaterEqual {
		return true, route.OperatorGreaterEqual
	} else if modOpr == ModOperLess {
		return true, route.OperatorLess
	} else if modOpr == ModOperLessEqual {
		return true, route.OperatorLessEqual
	} else {
		klog.Warningf("Mod operator [%v] is invalid", modOpr)
		return false, route.OperatorUnDefined
	}
}

func getMatchType(policyRule string, policy ingress.TrafficShapingPolicy) (route.OperatorType, route.MatchType) {
	ret := false
	modOpr := route.OperatorUnDefined
	matchType := route.WholeMatch
	modDivisor := policy.ModDivisor
	modRelationalOpr := policy.ModRelationalOpr
	modRemainder := policy.ModRemainder

	if len(policyRule) > 0 {
		matchType = route.StrListInMatch
	} else if checkModDivisor(modDivisor) && checkModRemainder(modDivisor, modRemainder) {
		ret, modOpr = getModOper(modRelationalOpr)
		if ret {
			matchType = route.ModCompare
		}
	}

	return modOpr, matchType
}

func getTagItem(policyRule string, maxValNum int, policy ingress.TrafficShapingPolicy) (route.MatchType, *route.TagItemCondition) {
	condition := &route.TagItemCondition{}
	operType, matchType := getMatchType(policyRule, policy)

	if matchType == route.ModCompare {
		condition.Divisor = policy.ModDivisor
		condition.Remainder = policy.ModRemainder
		condition.Operator = operType
	} else if matchType == route.StrListInMatch {
		strList := &route.TagValueStrList{}
		items := strings.Split(policyRule, CanaryDelimiter)
		num := 0
		for _, item := range items {
			if item == "" {
				continue
			}
			if num == maxValNum {
				klog.Errorf("Policy[%v] exceeds the value limit [%v], [%v] ignored",
					policyRule, maxValNum, item)
				break
			}
			strList.Value = append(strList.Value, item)
			num++
		}
		condition.ValueList = strList
	} else {
		condition.ValueStr = Always
	}

	return matchType, condition
}

func createCanary(seq int, locType route.LocationType, routeKey string, policyRule string, maxValNum int, cfg ngx_config.Configuration, server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) (*route.VirtualService, *route.TagRouter) {
	canaryService := &route.VirtualService{}
	tagRouter := &route.TagRouter{}
	policy := canary.TrafficShapingPolicy
	condition := &route.TagItemCondition{}
	matchType := route.MatchUnDefined

	pathCanaryServiceName := server.Hostname + loc.Path + Canary + strconv.Itoa(seq)
	canaryService = createVirtualService(cfg, canary.Target, loc, pathCanaryServiceName, server)
	createActions(cfg, canary, canaryService)
	matchType, condition = getTagItem(policyRule, maxValNum, policy)

	tagItem := &route.TagItem{
		Location:  locType,
		Key:       routeKey,
		Condition: condition,
		MatchType: matchType,
	}

	tagRule := &route.TagRule{
		Items: []*route.TagItem{
			tagItem,
		},
	}

	tagRouter = &route.TagRouter{
		ServiceName: pathCanaryServiceName,
		Rules: []*route.TagRule{
			tagRule,
		},
	}

	return canaryService, tagRouter
}

func createHeaderCanary(seq int, cfg ngx_config.Configuration, server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) (*route.VirtualService, *route.TagRouter) {
	policy := canary.TrafficShapingPolicy
	klog.Infof("Loc[%v%v], header=[%v], value=[%v], modDivisor=[%v], modOpr=[%v], modRemainder=[%v]",
		server.Hostname, loc.Path, policy.Header, policy.HeaderValue, policy.ModDivisor, policy.ModRelationalOpr, policy.ModRemainder)

	return createCanary(seq, route.LocHttpHeader, policy.Header, policy.HeaderValue, cfg.MaxCanaryHeaderValNum, cfg, server, loc, canary)
}

func createCookieCanary(seq int, cfg ngx_config.Configuration, server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) (*route.VirtualService, *route.TagRouter) {
	policy := canary.TrafficShapingPolicy
	klog.Infof("Loc[%v%v], cookie=[%v], value=[%v], modDivisor=[%v], modOpr=[%v], modRemainder=[%v]",
		server.Hostname, loc.Path, policy.Cookie, policy.CookieValue, policy.ModDivisor, policy.ModRelationalOpr, policy.ModRemainder)

	return createCanary(seq, route.LocHttpCookie, policy.Cookie, policy.CookieValue, cfg.MaxCanaryCookieValNum, cfg, server, loc, canary)
}

func createQueryCanary(seq int, cfg ngx_config.Configuration, server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) (*route.VirtualService, *route.TagRouter) {
	policy := canary.TrafficShapingPolicy
	klog.Infof("Loc[%v%v], query=[%v], value=[%v], modDivisor=[%v], modOpr=[%v], modRemainder=[%v]",
		server.Hostname, loc.Path, policy.Query, policy.QueryValue, policy.ModDivisor, policy.ModRelationalOpr, policy.ModRemainder)

	return createCanary(seq, route.LocHttpQuery, policy.Query, policy.QueryValue, cfg.MaxCanaryQueryValNum, cfg, server, loc, canary)
}
func createWeightCanary(server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) *route.Upstream {
	upstream := &route.Upstream{
		Target: canary.Target,
		Weight: uint32(canary.TrafficShapingPolicy.Weight),
	}

	klog.Infof("Loc[%v%v], upstream[%v], weight=[%v]",
		server.Hostname, loc.Path, upstream.Target, upstream.Weight)
	return upstream
}

func upstreamWeight(cfg ngx_config.Configuration, canaryUps []*route.Upstream, loc *ingress.Location, service *route.VirtualService) {
	var totalWeightTraffic uint32 = 0
	var canaryWeightSum uint32 = 0
	upstreams := make([]*route.Upstream, 0, len(canaryUps))
	for _, canaryUp := range canaryUps {
		canaryWeightSum += canaryUp.Weight
		upstreams = append(upstreams, canaryUp)
	}

	totalWeightTraffic = uint32(loc.WeightTotal)
	if totalWeightTraffic < cfg.DefaultCanaryWeightTotal || totalWeightTraffic > cfg.MaxCanaryWeightTotal {
		klog.Errorf("Total weight [%v] of canary traffic [%v] exceeds limit [%v,%v], use default[%v]",
			totalWeightTraffic, service.ServiceName, cfg.DefaultCanaryWeightTotal,
			cfg.MaxCanaryWeightTotal, cfg.DefaultCanaryWeightTotal)
		totalWeightTraffic = cfg.DefaultCanaryWeightTotal
	}

	srvWeight := totalWeightTraffic - canaryWeightSum
	if srvWeight < 0 {
		klog.Errorf("Total weight [%v] of canary traffic [%v] is larger than [%v]",
			canaryWeightSum, service.ServiceName, totalWeightTraffic)
	} else {
		service.Upstreams[0].Weight = srvWeight
		klog.Infof("The weight of traffic [%v] is [%v]",
			service.ServiceName, service.Upstreams[0].Weight)
		upstreams = append(upstreams, service.Upstreams[0])
	}

	service.Upstreams = upstreams
}
func setCanaryPriority(canaries *[]*ingress.Canary) {
	headerCanaries := make([]*ingress.Canary, 0)
	cookieCanaries := make([]*ingress.Canary, 0)
	queryCanaries := make([]*ingress.Canary, 0)
	weightCanaries := make([]*ingress.Canary, 0)

	for _, canary := range *canaries {
		policy := canary.TrafficShapingPolicy
		if len(policy.Header) > 0 {
			headerCanaries = append(headerCanaries, canary)
		} else if len(policy.Cookie) > 0 {
			cookieCanaries = append(cookieCanaries, canary)
		} else if len(policy.Query) > 0 {
			queryCanaries = append(queryCanaries, canary)
		} else if policy.Weight > 0 {
			weightCanaries = append(weightCanaries, canary)
		} else {
			continue
		}
	}

	*canaries = (*canaries)[:0]
	*canaries = append(*canaries, headerCanaries...)
	*canaries = append(*canaries, cookieCanaries...)
	*canaries = append(*canaries, queryCanaries...)
	*canaries = append(*canaries, weightCanaries...)

	for _, canary := range *canaries {
		policy := canary.TrafficShapingPolicy
		klog.Infof("Canary priority: Header[%v],HeaderValue[%v],Cookie[%v],CookieValue[%v],Query[%v],QueryValue[%v],Weight[%v],ModDivisor[%v],ModRelationalOpr[%v],ModRemainder[%v]",
			policy.Header,
			policy.HeaderValue,
			policy.Cookie,
			policy.CookieValue,
			policy.Query,
			policy.QueryValue,
			policy.Weight,
			policy.ModDivisor,
			policy.ModRelationalOpr,
			policy.ModRemainder)
	}
}
