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
	// The default total weight of traffic
	DefaultTotalWeightTraffic = 100
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

	// Matadata SSL protocols
	MetaSSLProtocols = "ssl-protocols"
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
)

var (
	sslProtocolVersions = map[string]uint64{
		"SSLv2":    2,
		"SSLv3":    768,
		"TLSv1":    769,
		"TLSv1.1":  770,
		"TLSv1.2":  771,
		"TLSv1.3":  772,
		"DTLSv1":   65279,
		"DTLSv1.2": 65277,
		"NTLS":     257,
	}
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
		rootServiceName := server.Hostname + RootLocation
		paths := make([]*route.PathRouter, 0, len(server.Locations))
		for _, loc := range server.Locations {
			upsService := loc.Service
			pathServiceName := server.Hostname + loc.Path
			if upsService == nil {
				continue
			}
			service := createVirtualService(loc.Backend, loc, pathServiceName)
			path := &route.PathRouter{
				Path:        loc.Path,
				ServiceName: pathServiceName,
			}

			if len(loc.Canaries) > 0 {
				tags := make([]*route.TagRouter, 0, len(loc.Canaries))
				canaryUps := make([]*route.Upstream, 0, len(loc.Canaries))
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
						canaryService, tagRouter = createHeaderCanary(i, server, loc, canary)
						tags = append(tags, tagRouter)
						services = append(services, canaryService)
					} else if policy.Weight > 0 {
						canaryUpstream := createWeightCanary(server, loc, canary)
						canaryUps = append(canaryUps, canaryUpstream)
					} else {
						klog.Errorf("Loc[%v%v] canary[header=%v,headerval=%v,weight=%v] is invalid, ignored",
							server.Hostname, loc.Path, policy.Header, policy.HeaderValue, policy.Weight)
						continue
					}
				}

				path.Tags = tags
				upstreamWeight(canaryUps, service)
			}
			paths = append(paths, path)
			services = append(services, service)
		}

		hostRouter := &route.HostRouter{
			Host:        server.Hostname,
			ServiceName: rootServiceName,
			Paths:       paths,
		}

		router := &route.Router{
			HostRouter: hostRouter,
		}
		routers = append(routers, router)
	}

	return &route.Config{
		Routers:  routers,
		Services: services,
	}
}

func createVirtualService(target string, loc *ingress.Location, serviceName string) *route.VirtualService {
	upstream := &route.Upstream{
		Target: target,
		Weight: DefaultTotalWeightTraffic,
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
		Metadata:   createMetaData(loc),
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
		&route.Metadata{
			Key:   MetaSSLProtocols,
			Value: convertSSLVer(server.SSLProtocols),
		},
	}
}

func createHeaderCanary(seq int, server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) (*route.VirtualService, *route.TagRouter) {
	canaryService := &route.VirtualService{}
	tagRouter := &route.TagRouter{}
	policy := canary.TrafficShapingPolicy

	headerValue := ""
	if len(policy.HeaderValue) > 0 {
		headerValue = policy.HeaderValue
	} else {
		headerValue = Always
	}

	pathCanaryServiceName := server.Hostname + loc.Path + Canary + strconv.Itoa(seq)
	klog.Infof("Loc[%v%v], canary[%v], header=[%v], value=[%v]",
		server.Hostname, loc.Path, pathCanaryServiceName, policy.Header, headerValue)
	canaryService = createVirtualService(canary.Target, loc, pathCanaryServiceName)

	tagItem := &route.TagItem{
		Location:  route.LocHttpHeader,
		Key:       policy.Header,
		Value:     headerValue,
		MatchType: route.WholeMatch,
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

func createWeightCanary(server *ingress.Server, loc *ingress.Location, canary *ingress.Canary) *route.Upstream {
	upstream := &route.Upstream{
		Target: canary.Target,
		Weight: uint32(canary.TrafficShapingPolicy.Weight),
	}

	klog.Infof("Loc[%v%v], upstream[%v], weight=[%v]",
		server.Hostname, loc.Path, upstream.Target, upstream.Weight)
	return upstream
}

func upstreamWeight(canaryUps []*route.Upstream, service *route.VirtualService) {
	var canaryWeightSum uint32 = 0
	upstreams := make([]*route.Upstream, 0, len(canaryUps))
	for _, canaryUp := range canaryUps {
		canaryWeightSum += canaryUp.Weight
		upstreams = append(upstreams, canaryUp)
	}

	srvWeight := DefaultTotalWeightTraffic - canaryWeightSum
	if srvWeight < 0 {
		klog.Errorf("Total weight [%v] of canary traffic [%v] is larger than [%v]",
			canaryWeightSum, service.ServiceName, DefaultTotalWeightTraffic)
	} else {
		service.Upstreams[0].Weight = srvWeight
		klog.Infof("The weight of traffic [%v] is [%v]",
			service.ServiceName, service.Upstreams[0].Weight)
		upstreams = append(upstreams, service.Upstreams[0])
	}

	service.Upstreams = upstreams
}

func convertSSLVer(sslProtocolStr string) string {
	sslVers := make([]string, 0)
	sslProtocols := strings.Fields(sslProtocolStr)
	for _, sslProtocol := range sslProtocols {
		sslVer, exists := sslProtocolVersions[sslProtocol]
		if !exists {
			klog.Errorf("Invalid SSL version [%v] in ssl-protocols [%v]",
				sslProtocol, sslProtocolStr)
			continue
		}

		sslVers = append(sslVers, strconv.FormatUint(sslVer, 10))
	}

	sslVersionStr := strings.Join(sslVers, " ")
	klog.Infof("ssl-protocols [%v] is converted to [%v]", sslProtocolStr, sslVersionStr)

	return sslVersionStr
}

func createCorsOriginRegex(corsOrigins []string) string {
	if len(corsOrigins) == 1 && corsOrigins[0] == "*" {
		return "*"
	}

	var originsRegex string = ""
	for i, origin := range corsOrigins {
		originTrimmed := strings.TrimSpace(origin)
		if len(originTrimmed) > 0 {
			originsRegex += originTrimmed
			if i != len(corsOrigins)-1 {
				originsRegex = originsRegex + ", "
			}
		}
	}

	return originsRegex
}
