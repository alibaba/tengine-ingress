/*
Copyright 2015 The Kubernetes Authors.
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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/hashstructure"
	apiv1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	ingcheckclient "k8s.io/ingress-nginx/internal/checksum/ingress/client/clientset/versioned"
	secretcheckclient "k8s.io/ingress-nginx/internal/checksum/secret/client/clientset/versioned"
	"k8s.io/ingress-nginx/internal/ingress"
	"k8s.io/ingress-nginx/internal/ingress/annotations"
	"k8s.io/ingress-nginx/internal/ingress/annotations/class"
	"k8s.io/ingress-nginx/internal/ingress/annotations/log"
	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/annotations/proxy"
	ngx_config "k8s.io/ingress-nginx/internal/ingress/controller/config"
	"k8s.io/ingress-nginx/internal/ingress/inspector"
	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/internal/lock"
	"k8s.io/ingress-nginx/internal/nginx"
	"k8s.io/klog"
)

const (
	defUpstreamName = "upstream-default-backend"
	defServerName   = "_"
	rootLocation    = "/"
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost string
	RootCAFile    string

	KubeConfigFile string

	Client            clientset.Interface
	ClientIng         clientset.Interface
	ClientIngCheck    ingcheckclient.Interface
	ClientSecretCheck secretcheckclient.Interface

	ResyncPeriod time.Duration

	ConfigMapName  string
	DefaultService string

	Namespace string

	// +optional
	TCPConfigMapName string
	// +optional
	UDPConfigMapName string

	DefaultSSLCertificate string

	// +optional
	PublishService       string
	PublishStatusAddress string

	UpdateStatus           bool
	UseNodeInternalIP      bool
	ElectionID             string
	UpdateStatusOnShutdown bool

	ListenPorts *ngx_config.ListenPorts

	EnableSSLPassthrough bool

	EnableProfiling bool

	EnableMetrics  bool
	MetricsPerHost bool

	FakeCertificate *ingress.SSLCert

	SyncRateLimit float32

	DisableCatchAll bool

	ValidationWebhook         string
	ValidationWebhookCertPath string
	ValidationWebhookKeyPath  string

	GlobalExternalAuth *ngx_config.GlobalExternalAuth
}

// GetPublishService returns the Service used to set the load-balancer status of Ingresses.
func (n NGINXController) GetPublishService() *apiv1.Service {
	s, err := n.store.GetService(n.cfg.PublishService)
	if err != nil {
		return nil
	}

	return s
}

// syncIngress collects all the pieces required to assemble the NGINX
// configuration file and passes the resulting data structures to the backend
// (OnUpdate) when a reload is deemed necessary.
func (n *NGINXController) syncIngress(interface{}) error {
	n.syncRateLimiter.Accept()

	if n.syncQueue.IsShuttingDown() {
		return nil
	}

	ings := n.store.ListIngresses(nil)
	ready, err0 := ingCheck(n.store.ListIngsWithAnnotation(), n.store.ListLocalIngressCheckSums(nil))
	cfg := n.store.GetBackendConfiguration()
	if ready {
		n.checksumStatus.IngChecksumStatus = true
		n.metricCollector.IncIngChecksumCount()
		n.metricCollector.ClearIngChecksumErrorCount()
	} else if err0 != nil {
		n.checksumStatus.IngChecksumStatus = false
		if lock.IsFileExists(cfg.StatusTengineFilePath) {
			klog.Errorf("Ingress ID mismatch and [%v] exists, alarm:\n\n%v", cfg.StatusTengineFilePath, err0)
			n.metricCollector.IncIngChecksumErrorCount()
		} else {
			klog.Infof("Ingress ID mismatch and [%v] does NOT exist, ignoring alarm:\n\n%v", cfg.StatusTengineFilePath, err0)
		}
		return err0
	}

	hosts, servers, pcfg := n.getConfiguration(ings)
	n.metricCollector.SetSSLExpireTime(servers)

	if n.runningConfig.Equal(pcfg) {
		klog.Infof("No configuration change detected, skipping hot reload.")
		return nil
	}

	klog.Infof("Configuration changes detected.")

	n.metricCollector.SetHosts(hosts)
	hash, _ := hashstructure.Hash(pcfg, &hashstructure.HashOptions{
		TagName: "json",
	})

	pcfg.ConfigurationChecksum = fmt.Sprintf("%v", hash)
	err := n.OnUpdate(*pcfg)
	if err != nil {
		n.metricCollector.IncReloadErrorCount()
		n.metricCollector.ConfigSuccess(hash, false)
		klog.Errorf("Unexpected failure reloading the backend:\n%v", err)
		return err
	}

	md5, err := hotReload(n.hotReloadMD5, cfg, *pcfg, false)
	if err != nil {
		klog.Errorf("Hot reloading failed:\n%v", err)
		return err
	}
	n.hotReloadMD5 = md5

	n.metricCollector.ConfigSuccess(hash, true)
	n.metricCollector.IncReloadCount()

	isFirstSync := n.runningConfig.Equal(&ingress.Configuration{})
	if isFirstSync {
		// For the initial sync it always takes some time for NGINX to start listening
		// For large configurations it might take a while so we loop and back off
		klog.Info("Initial sync, sleeping for 1 second.")
		time.Sleep(1 * time.Second)
	}

	retry := wait.Backoff{
		Steps:    15,
		Duration: 1 * time.Second,
		Factor:   0.8,
		Jitter:   0.1,
	}

	err = wait.ExponentialBackoff(retry, func() (bool, error) {
		err := n.configureDynamically(pcfg)
		if err == nil {
			klog.V(2).Infof("Dynamic reconfiguration succeeded.")
			return true, nil
		}

		klog.Warningf("Dynamic reconfiguration failed: %v", err)
		return false, err
	})
	if err != nil {
		klog.Errorf("Unexpected failure reconfiguring NGINX:\n%v", err)
		return err
	}

	ri := getRemovedIngresses(n.runningConfig, pcfg)
	re := getRemovedHosts(n.runningConfig, pcfg)
	n.metricCollector.RemoveMetrics(ri, re)

	n.runningConfig = pcfg
	f, _ := lock.CreateDirFile(cfg.StatusTengineFilePath)
	defer f.Close()

	return nil
}

// CheckIngress returns an error in case the provided ingress, when added
// to the current configuration, generates an invalid configuration
func (n *NGINXController) CheckIngress(ing *networking.Ingress) error {
	//TODO: this is wrong
	if n == nil {
		return fmt.Errorf("cannot check ingress on a nil ingress controller")
	}

	if ing == nil {
		// no ingress to add, no state change
		return nil
	}

	if !class.IsValid(ing) {
		klog.Infof("ignoring ingress %v in %v based on annotation %v", ing.Name, ing.ObjectMeta.Namespace, class.IngressKey)
		return nil
	}

	if n.cfg.Namespace != "" && ing.ObjectMeta.Namespace != n.cfg.Namespace {
		klog.Infof("ignoring ingress %v in namespace %v different from the namespace watched %s", ing.Name, ing.ObjectMeta.Namespace, n.cfg.Namespace)
		return nil
	}

	filter := func(toCheck *ingress.Ingress) bool {
		return toCheck.ObjectMeta.Namespace == ing.ObjectMeta.Namespace &&
			toCheck.ObjectMeta.Name == ing.ObjectMeta.Name
	}

	ings := n.store.ListIngresses(filter)
	ings = append(ings, &ingress.Ingress{
		Ingress:           *ing,
		ParsedAnnotations: annotations.NewAnnotationExtractor(n.store).Extract(ing),
	})

	_, _, pcfg := n.getConfiguration(ings)

	cfg := n.store.GetBackendConfiguration()
	cfg.Resolver = n.resolver

	content, err := n.generateTemplate(cfg, *pcfg)
	if err != nil {
		n.metricCollector.IncCheckErrorCount(ing.ObjectMeta.Namespace, ing.Name)
		return err
	}

	err = n.testTemplate(content)
	if err != nil {
		n.metricCollector.IncCheckErrorCount(ing.ObjectMeta.Namespace, ing.Name)
	} else {
		n.metricCollector.IncCheckCount(ing.ObjectMeta.Namespace, ing.Name)
	}

	return err
}

// GetWarnings returns a list of warnings an Ingress gets when being created.
// The warnings are going to be used in an admission webhook, and they represent
// a list of messages that users need to be aware (like deprecation notices)
// when creating a new ingress object
func (n *NGINXController) CheckWarning(ing *networking.Ingress) ([]string, error) {
	warnings := make([]string, 0)

	var deprecatedAnnotations = sets.NewString()
	deprecatedAnnotations.Insert(
		"enable-influxdb",
		"influxdb-measurement",
		"influxdb-port",
		"influxdb-host",
		"influxdb-server-name",
		"secure-verify-ca-secret",
	)

	// Skip checks if the ingress is marked as deleted
	if !ing.DeletionTimestamp.IsZero() {
		return warnings, nil
	}

	anns := ing.GetAnnotations()
	for k := range anns {
		trimmedkey := strings.TrimPrefix(k, parser.AnnotationsPrefix+"/")
		if deprecatedAnnotations.Has(trimmedkey) {
			warnings = append(warnings, fmt.Sprintf("annotation %s is deprecated", k))
		}
	}

	// Add each validation as a single warning
	// rikatz: I know this is somehow a duplicated code from CheckIngress, but my goal was to deliver fast warning on this behavior. We
	// can and should, tho, simplify this in the near future
	if err := inspector.ValidatePathType(ing); err != nil {
		if errs, is := err.(interface{ Unwrap() []error }); is {
			for _, errW := range errs.Unwrap() {
				warnings = append(warnings, errW.Error())
			}
		} else {
			warnings = append(warnings, err.Error())
		}
	}

	return warnings, nil
}

func (n *NGINXController) getStreamServices(configmapName string, proto apiv1.Protocol) []ingress.L4Service {
	if configmapName == "" {
		return []ingress.L4Service{}
	}
	klog.V(3).Infof("Obtaining information about %v stream services from ConfigMap %q", proto, configmapName)
	_, _, err := k8s.ParseNameNS(configmapName)
	if err != nil {
		klog.Warningf("Error parsing ConfigMap reference %q: %v", configmapName, err)
		return []ingress.L4Service{}
	}
	configmap, err := n.store.GetConfigMap(configmapName)
	if err != nil {
		klog.Warningf("Error getting ConfigMap %q: %v", configmapName, err)
		return []ingress.L4Service{}
	}

	var svcs []ingress.L4Service
	var svcProxyProtocol ingress.ProxyProtocol

	rp := []int{
		n.cfg.ListenPorts.HTTP,
		n.cfg.ListenPorts.HTTPS,
		n.cfg.ListenPorts.QUIC,
		n.cfg.ListenPorts.SSLProxy,
		n.cfg.ListenPorts.Health,
		n.cfg.ListenPorts.Default,
		nginx.ProfilerPort,
		nginx.StatusPort,
		nginx.StreamPort,
	}

	reserverdPorts := sets.NewInt(rp...)
	// svcRef format: <(str)namespace>/<(str)service>:<(intstr)port>[:<("PROXY")decode>:<("PROXY")encode>]
	for port, svcRef := range configmap.Data {
		externalPort, err := strconv.Atoi(port)
		if err != nil {
			klog.Warningf("%q is not a valid %v port number", port, proto)
			continue
		}
		if reserverdPorts.Has(externalPort) {
			klog.Warningf("Port %d cannot be used for %v stream services. It is reserved for the Ingress controller.", externalPort, proto)
			continue
		}
		nsSvcPort := strings.Split(svcRef, ":")
		if len(nsSvcPort) < 2 {
			klog.Warningf("Invalid Service reference %q for %v port %d", svcRef, proto, externalPort)
			continue
		}
		nsName := nsSvcPort[0]
		svcPort := nsSvcPort[1]
		svcProxyProtocol.Decode = false
		svcProxyProtocol.Encode = false
		// Proxy Protocol is only compatible with TCP Services
		if len(nsSvcPort) >= 3 && proto == apiv1.ProtocolTCP {
			if len(nsSvcPort) >= 3 && strings.ToUpper(nsSvcPort[2]) == "PROXY" {
				svcProxyProtocol.Decode = true
			}
			if len(nsSvcPort) == 4 && strings.ToUpper(nsSvcPort[3]) == "PROXY" {
				svcProxyProtocol.Encode = true
			}
		}
		svcNs, svcName, err := k8s.ParseNameNS(nsName)
		if err != nil {
			klog.Warningf("%v", err)
			continue
		}
		svc, err := n.store.GetService(nsName)
		if err != nil {
			klog.Warningf("Error getting Service %q: %v", nsName, err)
			continue
		}
		var endps []ingress.Endpoint
		targetPort, err := strconv.Atoi(svcPort)
		if err != nil {
			// not a port number, fall back to using port name
			klog.V(3).Infof("Searching Endpoints with %v port name %q for Service %q", proto, svcPort, nsName)
			for _, sp := range svc.Spec.Ports {
				if sp.Name == svcPort {
					if sp.Protocol == proto {
						endps = getEndpoints(svc, &sp, proto, n.store.GetServiceEndpoints)
						break
					}
				}
			}
		} else {
			klog.V(3).Infof("Searching Endpoints with %v port number %d for Service %q", proto, targetPort, nsName)
			for _, sp := range svc.Spec.Ports {
				if sp.Port == int32(targetPort) {
					if sp.Protocol == proto {
						endps = getEndpoints(svc, &sp, proto, n.store.GetServiceEndpoints)
						break
					}
				}
			}
		}
		// stream services cannot contain empty upstreams and there is
		// no default backend equivalent
		if len(endps) == 0 {
			klog.Warningf("Service %q does not have any active Endpoint for %v port %v", nsName, proto, svcPort)
			continue
		}
		svcs = append(svcs, ingress.L4Service{
			Port: externalPort,
			Backend: ingress.L4Backend{
				Name:          svcName,
				Namespace:     svcNs,
				Port:          intstr.FromString(svcPort),
				Protocol:      proto,
				ProxyProtocol: svcProxyProtocol,
			},
			Endpoints: endps,
			Service:   svc,
		})
	}
	// Keep upstream order sorted to reduce unnecessary nginx config reloads.
	sort.SliceStable(svcs, func(i, j int) bool {
		return svcs[i].Port < svcs[j].Port
	})
	return svcs
}

// getDefaultUpstream returns the upstream associated with the default backend.
// Configures the upstream to return HTTP code 503 in case of error.
func (n *NGINXController) getDefaultUpstream() *ingress.Backend {
	upstream := &ingress.Backend{
		Name: defUpstreamName,
	}
	svcKey := n.cfg.DefaultService

	if len(svcKey) == 0 {
		upstream.Endpoints = append(upstream.Endpoints, n.DefaultEndpoint())
		return upstream
	}

	svc, err := n.store.GetService(svcKey)
	if err != nil {
		klog.Warningf("Error getting default backend %q: %v", svcKey, err)
		upstream.Endpoints = append(upstream.Endpoints, n.DefaultEndpoint())
		return upstream
	}

	endps := getEndpoints(svc, &svc.Spec.Ports[0], apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
	if len(endps) == 0 {
		klog.Warningf("Service %q does not have any active Endpoint", svcKey)
		endps = []ingress.Endpoint{n.DefaultEndpoint()}
	}

	upstream.Service = svc
	upstream.Endpoints = append(upstream.Endpoints, endps...)
	return upstream
}

// getConfiguration returns the configuration matching the standard kubernetes ingress
func (n *NGINXController) getConfiguration(ingresses []*ingress.Ingress) (sets.String, []*ingress.Server, *ingress.Configuration) {

	upstreams, servers := n.getBackendServers(ingresses)
	var passUpstreams []*ingress.SSLPassthroughBackend

	hosts := sets.NewString()

	for _, server := range servers {
		if !hosts.Has(server.Hostname) {
			hosts.Insert(server.Hostname)
		}

		for _, alias := range server.Aliases {
			if !hosts.Has(alias) {
				hosts.Insert(alias)
			}
		}

		if !server.SSLPassthrough {
			continue
		}

		for _, loc := range server.Locations {
			if loc.Path != rootLocation {
				klog.Warningf("Ignoring SSL Passthrough for location %q in server %q", loc.Path, server.Hostname)
				continue
			}
			passUpstreams = append(passUpstreams, &ingress.SSLPassthroughBackend{
				Backend:  loc.Backend,
				Hostname: server.Hostname,
				Service:  loc.Service,
				Port:     loc.Port,
			})
			break
		}
	}

	return hosts, servers, &ingress.Configuration{
		Backends:              upstreams,
		Servers:               servers,
		TCPEndpoints:          n.getStreamServices(n.cfg.TCPConfigMapName, apiv1.ProtocolTCP),
		UDPEndpoints:          n.getStreamServices(n.cfg.UDPConfigMapName, apiv1.ProtocolUDP),
		PassthroughBackends:   passUpstreams,
		BackendConfigChecksum: n.store.GetBackendConfiguration().Checksum,
		ControllerPodsCount:   n.store.GetRunningControllerPodsCount(),
	}
}

// getBackendServers returns a list of Upstream and Server to be used by the
// backend.  An upstream can be used in multiple servers if the namespace,
// service name and port are the same.
func (n *NGINXController) getBackendServers(ingresses []*ingress.Ingress) ([]*ingress.Backend, []*ingress.Server) {
	du := n.getDefaultUpstream()
	upstreams := n.createUpstreams(ingresses, du)
	servers := n.createServers(ingresses, upstreams, du)

	var canaryIngresses []*ingress.Ingress

	for _, ing := range ingresses {

		ingKey := k8s.MetaNamespaceKey(ing)
		anns := ing.ParsedAnnotations

		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = defServerName
			}

			server := servers[host]
			if server == nil {
				server = servers[defServerName]
			}

			if rule.HTTP == nil &&
				host != defServerName {
				klog.V(3).Infof("Ingress %q does not contain any HTTP rule, using default backend", ingKey)
				continue
			}

			if server.AuthTLSError == "" && anns.CertificateAuth.AuthTLSError != "" {
				server.AuthTLSError = anns.CertificateAuth.AuthTLSError
			}

			if server.CertificateAuth.CAFileName == "" {
				server.CertificateAuth = anns.CertificateAuth
				if server.CertificateAuth.Secret != "" && server.CertificateAuth.CAFileName == "" {
					klog.V(3).Infof("Secret %q has no 'ca.crt' key, mutual authentication disabled for Ingress %q",
						server.CertificateAuth.Secret, ingKey)
				}
			} else {
				klog.V(3).Infof("Server %q is already configured for mutual authentication (Ingress %q)",
					server.Hostname, ingKey)
			}

			if server.ProxySSL.CAFileName == "" {
				server.ProxySSL = anns.ProxySSL
				if server.ProxySSL.Secret != "" && server.ProxySSL.CAFileName == "" {
					klog.V(3).Infof("Secret %q has no 'ca.crt' key, client cert authentication disabled for Ingress %q",
						server.ProxySSL.Secret, ingKey)
				}
			} else {
				klog.V(3).Infof("Server %q is already configured for client cert authentication (Ingress %q)",
					server.Hostname, ingKey)
			}

			if rule.HTTP == nil {
				klog.V(3).Infof("Ingress %q does not contain any HTTP rule, using default backend", ingKey)
				continue
			}

			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					// skip non-service backends
					klog.V(3).Infof("Ingress %q and path %q does not contain a service backend, using default backend", ingKey, path.Path)
					continue
				}

				upsName := upstreamName(ing.Namespace, path.Backend.Service)
				ups := upstreams[upsName]

				// Backend is not referenced to by a server
				if ups.NoServer {
					continue
				}

				nginxPath := rootLocation
				if path.Path != "" {
					nginxPath = path.Path
				}

				addLoc := true
				for _, loc := range server.Locations {
					if loc.Path == nginxPath {
						addLoc = false

						if !loc.IsDefBackend {
							klog.V(3).Infof("Location %q already configured for server %q with upstream %q (Ingress %q)",
								loc.Path, server.Hostname, loc.Backend, ingKey)
							break
						}

						klog.V(3).Infof("Replacing location %q for server %q with upstream %q to use upstream %q (Ingress %q)",
							loc.Path, server.Hostname, loc.Backend, ups.Name, ingKey)

						loc.Backend = ups.Name
						loc.IsDefBackend = false
						loc.Port = ups.Port
						loc.Service = ups.Service
						loc.Ingress = ing
						locationApplyAnnotations(loc, anns)

						if loc.Redirect.FromToWWW {
							server.RedirectFromToWWW = true
						}
						break
					}
				}

				// new location
				if addLoc {
					klog.V(3).Infof("Adding location %q for server %q with upstream %q (Ingress %q)",
						nginxPath, server.Hostname, ups.Name, ingKey)

					loc := &ingress.Location{
						Path:         nginxPath,
						Backend:      ups.Name,
						IsDefBackend: false,
						Service:      ups.Service,
						Port:         ups.Port,
						Ingress:      ing,
					}
					locationApplyAnnotations(loc, anns)

					if loc.Redirect.FromToWWW {
						server.RedirectFromToWWW = true
					}

					server.Locations = append(server.Locations, loc)
				}

				if ups.SessionAffinity.AffinityType == "" {
					ups.SessionAffinity.AffinityType = anns.SessionAffinity.Type
				}

				if ups.SessionAffinity.AffinityMode == "" {
					ups.SessionAffinity.AffinityMode = anns.SessionAffinity.Mode
				}

				if anns.SessionAffinity.Type == "cookie" {
					cookiePath := anns.SessionAffinity.Cookie.Path
					if anns.Rewrite.UseRegex && cookiePath == "" {
						klog.Warningf("session-cookie-path should be set when use-regex is true")
					}

					ups.SessionAffinity.CookieSessionAffinity.Name = anns.SessionAffinity.Cookie.Name
					ups.SessionAffinity.CookieSessionAffinity.Expires = anns.SessionAffinity.Cookie.Expires
					ups.SessionAffinity.CookieSessionAffinity.MaxAge = anns.SessionAffinity.Cookie.MaxAge
					ups.SessionAffinity.CookieSessionAffinity.Path = cookiePath
					ups.SessionAffinity.CookieSessionAffinity.SameSite = anns.SessionAffinity.Cookie.SameSite
					ups.SessionAffinity.CookieSessionAffinity.ConditionalSameSiteNone = anns.SessionAffinity.Cookie.ConditionalSameSiteNone
					ups.SessionAffinity.CookieSessionAffinity.ChangeOnFailure = anns.SessionAffinity.Cookie.ChangeOnFailure

					locs := ups.SessionAffinity.CookieSessionAffinity.Locations
					if _, ok := locs[host]; !ok {
						locs[host] = []string{}
					}
					locs[host] = append(locs[host], path.Path)
				}
			}
		}

		// set aside canary ingresses to merge later
		if anns.Canary.Enabled && n.verifyCanaryReferrer(ingKey, anns) {
			canaryIngresses = append(canaryIngresses, ing)
		}
	}

	if nonCanaryIngressExists(ingresses, canaryIngresses) {
		for _, canaryIng := range canaryIngresses {
			n.mergeAlternativeBackends(canaryIng, upstreams, servers)
		}
	}

	aUpstreams := make([]*ingress.Backend, 0, len(upstreams))

	if !n.store.GetBackendConfiguration().UseCustomDefBackend {
		for _, upstream := range upstreams {
			aUpstreams = append(aUpstreams, upstream)
		}
	} else {
		//Add config for LocationDefaultBackend
		for _, upstream := range upstreams {
			aUpstreams = append(aUpstreams, upstream)

			if upstream.Name == defUpstreamName {
				continue
			}

			isHTTPSfrom := []*ingress.Server{}
			for _, server := range servers {
				for _, location := range server.Locations {
					// use default backend
					if !shouldCreateUpstreamForLocationDefaultBackend(upstream, location) {
						continue
					}

					sp := location.DefaultBackend.Spec.Ports[0]
					endps := getEndpoints(location.DefaultBackend, &sp, apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
					// custom backend is valid only if contains at least one endpoint
					if len(endps) > 0 {
						name := fmt.Sprintf("custom-default-backend-%v", location.DefaultBackend.GetName())
						klog.V(3).Infof("Creating \"%v\" upstream based on default backend annotation", name)

						nb := upstream.DeepCopy()
						nb.Name = name
						nb.Endpoints = endps
						aUpstreams = append(aUpstreams, nb)
						location.DefaultBackendUpstreamName = name

						if len(upstream.Endpoints) == 0 {
							klog.V(3).Infof("Upstream %q has no active Endpoint, so using custom default backend for location %q in server %q (Service \"%v/%v\")",
								upstream.Name, location.Path, server.Hostname, location.DefaultBackend.Namespace, location.DefaultBackend.Name)

							location.Backend = name
						}
					}

					if server.SSLPassthrough {
						if location.Path == rootLocation {
							if location.Backend == defUpstreamName {
								klog.Warningf("Server %q has no default backend, ignoring SSL Passthrough.", server.Hostname)
								continue
							}
							isHTTPSfrom = append(isHTTPSfrom, server)
						}
					}
				}
			}

			if len(isHTTPSfrom) > 0 {
				upstream.SSLPassthrough = true
			}
		}
	}

	aServers := make([]*ingress.Server, 0, len(servers))
	for _, value := range servers {
		sort.SliceStable(value.Locations, func(i, j int) bool {
			return value.Locations[i].Path > value.Locations[j].Path
		})

		sort.SliceStable(value.Locations, func(i, j int) bool {
			return len(value.Locations[i].Path) > len(value.Locations[j].Path)
		})

		sort.SliceStable(value.SSLCerts, func(i, j int) bool {
			return value.SSLCerts[i].Name > value.SSLCerts[j].Name
		})

		sort.SliceStable(value.SSLCerts, func(i, j int) bool {
			return len(value.SSLCerts[i].Name) > len(value.SSLCerts[j].Name)
		})
		aServers = append(aServers, value)
	}

	sort.SliceStable(aUpstreams, func(a, b int) bool {
		return aUpstreams[a].Name < aUpstreams[b].Name
	})

	sort.SliceStable(aServers, func(i, j int) bool {
		return aServers[i].Hostname < aServers[j].Hostname
	})

	return aUpstreams, aServers
}

// createUpstreams creates the NGINX upstreams (Endpoints) for each Service
// referenced in Ingress rules.
func (n *NGINXController) createUpstreams(data []*ingress.Ingress, du *ingress.Backend) map[string]*ingress.Backend {
	upstreams := make(map[string]*ingress.Backend)
	upstreams[defUpstreamName] = du

	for _, ing := range data {
		ingKey := k8s.MetaNamespaceKey(ing)
		anns := ing.ParsedAnnotations

		var defBackend string
		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			defBackend = upstreamName(ing.Namespace, ing.Spec.DefaultBackend.Service)

			klog.V(3).Infof("Creating upstream %q", defBackend)
			upstreams[defBackend] = newUpstream(defBackend)

			upstreams[defBackend].UpstreamHashBy.UpstreamHashBy = anns.UpstreamHashBy.UpstreamHashBy
			upstreams[defBackend].UpstreamHashBy.UpstreamHashBySubset = anns.UpstreamHashBy.UpstreamHashBySubset
			upstreams[defBackend].UpstreamHashBy.UpstreamHashBySubsetSize = anns.UpstreamHashBy.UpstreamHashBySubsetSize

			upstreams[defBackend].LoadBalancing = anns.LoadBalancing
			if upstreams[defBackend].LoadBalancing == "" {
				upstreams[defBackend].LoadBalancing = n.store.GetBackendConfiguration().LoadBalancing
			}

			svcKey := fmt.Sprintf("%v/%v", ing.Namespace, ing.Spec.DefaultBackend.Service.Name)
			// add the service ClusterIP as a single Endpoint instead of individual Endpoints
			if anns.ServiceUpstream {
				endpoint, err := n.getServiceClusterEndpoint(svcKey, ing.Spec.DefaultBackend)
				if err != nil {
					klog.Errorf("Failed to determine a suitable ClusterIP Endpoint for Service %q: %v", svcKey, err)
				} else {
					upstreams[defBackend].Endpoints = []ingress.Endpoint{endpoint}
				}
			}

			// configure traffic shaping for canary
			if anns.Canary.Enabled && n.verifyCanaryReferrer(ingKey, anns) {
				upstreams[defBackend].NoServer = true
				upstreams[defBackend].TrafficShapingPolicy = ingress.TrafficShapingPolicy{
					Weight:      anns.Canary.Weight,
					Header:      anns.Canary.Header,
					HeaderValue: anns.Canary.HeaderValue,
					Cookie:      anns.Canary.Cookie,
				}
			}

			if len(upstreams[defBackend].Endpoints) == 0 {
				_, port := upstreamServiceNameAndPort(ing.Spec.DefaultBackend.Service)
				endps, err := n.serviceEndpoints(svcKey, port.String())
				upstreams[defBackend].Endpoints = append(upstreams[defBackend].Endpoints, endps...)
				if err != nil {
					klog.Warningf("Error creating upstream %q: %v", defBackend, err)
				}
			}

			s, err := n.store.GetService(svcKey)
			if err != nil {
				klog.Warningf("Error obtaining Service %q: %v", svcKey, err)
			}
			upstreams[defBackend].Service = s
		}

		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}

			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					// skip non-service backends
					klog.V(3).Infof("Ingress %q and path %q does not contain a service backend, using default backend", ingKey, path.Path)
					continue
				}

				name := upstreamName(ing.Namespace, path.Backend.Service)
				svcName, svcPort := upstreamServiceNameAndPort(path.Backend.Service)
				if _, ok := upstreams[name]; ok {
					continue
				}

				klog.V(3).Infof("Creating upstream %q", name)
				upstreams[name] = newUpstream(name)
				upstreams[name].Port = svcPort

				upstreams[name].UpstreamHashBy.UpstreamHashBy = anns.UpstreamHashBy.UpstreamHashBy
				upstreams[name].UpstreamHashBy.UpstreamHashBySubset = anns.UpstreamHashBy.UpstreamHashBySubset
				upstreams[name].UpstreamHashBy.UpstreamHashBySubsetSize = anns.UpstreamHashBy.UpstreamHashBySubsetSize

				upstreams[name].LoadBalancing = anns.LoadBalancing
				if upstreams[name].LoadBalancing == "" {
					upstreams[name].LoadBalancing = n.store.GetBackendConfiguration().LoadBalancing
				}

				svcKey := fmt.Sprintf("%v/%v", ing.Namespace, svcName)
				// add the service ClusterIP as a single Endpoint instead of individual Endpoints
				if anns.ServiceUpstream {
					endpoint, err := n.getServiceClusterEndpoint(svcKey, &path.Backend)
					if err != nil {
						klog.Errorf("Failed to determine a suitable ClusterIP Endpoint for Service %q: %v", svcKey, err)
					} else {
						upstreams[name].Endpoints = []ingress.Endpoint{endpoint}
					}
				}

				// configure traffic shaping for canary
				if anns.Canary.Enabled && n.verifyCanaryReferrer(ingKey, anns) {
					upstreams[name].NoServer = true
					upstreams[name].TrafficShapingPolicy = ingress.TrafficShapingPolicy{
						Weight:      anns.Canary.Weight,
						Header:      anns.Canary.Header,
						HeaderValue: anns.Canary.HeaderValue,
						Cookie:      anns.Canary.Cookie,
					}
				}

				if len(upstreams[name].Endpoints) == 0 {
					_, port := upstreamServiceNameAndPort(path.Backend.Service)
					endp, err := n.serviceEndpoints(svcKey, port.String())
					if err != nil {
						klog.Warningf("Error obtaining Endpoints for Service %q: %v", svcKey, err)
						continue
					}
					upstreams[name].Endpoints = endp
				}

				s, err := n.store.GetService(svcKey)
				if err != nil {
					klog.Warningf("Error obtaining Service %q: %v", svcKey, err)
					continue
				}

				upstreams[name].Service = s
			}
		}
	}

	return upstreams
}

// getServiceClusterEndpoint returns an Endpoint corresponding to the ClusterIP
// field of a Service.
func (n *NGINXController) getServiceClusterEndpoint(svcKey string, backend *networking.IngressBackend) (endpoint ingress.Endpoint, err error) {
	svc, err := n.store.GetService(svcKey)
	if err != nil {
		return endpoint, fmt.Errorf("service %q does not exist", svcKey)
	}

	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		return endpoint, fmt.Errorf("no ClusterIP found for Service %q", svcKey)
	}

	endpoint.Address = svc.Spec.ClusterIP

	// if the Service port is referenced by name in the Ingress, lookup the
	// actual port in the service spec
	if backend.Service != nil {
		_, svcportintorstr := upstreamServiceNameAndPort(backend.Service)
		if svcportintorstr.Type == intstr.String {
			var port int32 = -1
			for _, svcPort := range svc.Spec.Ports {
				if svcPort.Name == svcportintorstr.String() {
					port = svcPort.Port
					break
				}
			}
			if port == -1 {
				return endpoint, fmt.Errorf("service %q does not have a port named %q", svc.Name, svcportintorstr.String())
			}
			endpoint.Port = fmt.Sprintf("%d", port)
		} else {
			endpoint.Port = svcportintorstr.String()
		}
	}

	return endpoint, err
}

// serviceEndpoints returns the upstream servers (Endpoints) associated with a Service.
func (n *NGINXController) serviceEndpoints(svcKey, backendPort string) ([]ingress.Endpoint, error) {
	var upstreams []ingress.Endpoint

	svc, err := n.store.GetService(svcKey)
	if err != nil {
		return upstreams, err
	}

	klog.V(3).Infof("Obtaining ports information for Service %q", svcKey)

	// Ingress with an ExternalName Service and no port defined for that Service
	if svc.Spec.Type == apiv1.ServiceTypeExternalName {
		servicePort := externalNamePorts(backendPort, svc)
		endps := getEndpoints(svc, servicePort, apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
		if len(endps) == 0 {
			klog.Warningf("Service %q does not have any active Endpoint.", svcKey)
			return upstreams, nil
		}

		upstreams = append(upstreams, endps...)
		return upstreams, nil
	}

	for _, servicePort := range svc.Spec.Ports {
		// targetPort could be a string, use either the port name or number (int)
		if strconv.Itoa(int(servicePort.Port)) == backendPort ||
			servicePort.TargetPort.String() == backendPort ||
			servicePort.Name == backendPort {

			endps := getEndpoints(svc, &servicePort, apiv1.ProtocolTCP, n.store.GetServiceEndpoints)
			if len(endps) == 0 {
				klog.Warningf("Service %q does not have any active Endpoint.", svcKey)
			}

			upstreams = append(upstreams, endps...)
			break
		}
	}

	return upstreams, nil
}

func (n *NGINXController) getDefaultSSLCertificate() *ingress.SSLCert {
	// read custom default SSL certificate, fall back to generated default certificate
	if n.cfg.DefaultSSLCertificate != "" {
		certificate, err := n.store.GetLocalSSLCert(n.cfg.DefaultSSLCertificate)
		if err == nil {
			return certificate
		}

		klog.Warningf("Error loading custom default certificate, falling back to generated default:\n%v", err)
	}

	return n.cfg.FakeCertificate
}

// createServers builds a map of host name to Server structs from a map of
// already computed Upstream structs. Each Server is configured with at least
// one root location, which uses a default backend if left unspecified.
func (n *NGINXController) createServers(data []*ingress.Ingress,
	upstreams map[string]*ingress.Backend,
	du *ingress.Backend) map[string]*ingress.Server {

	servers := make(map[string]*ingress.Server, len(data))
	allAliases := make(map[string][]string, len(data))

	bdef := n.store.GetDefaultBackend()
	ngxProxy := proxy.Config{
		BodySize:             bdef.ProxyBodySize,
		ConnectTimeout:       bdef.ProxyConnectTimeout,
		SendTimeout:          bdef.ProxySendTimeout,
		ReadTimeout:          bdef.ProxyReadTimeout,
		BuffersNumber:        bdef.ProxyBuffersNumber,
		BufferSize:           bdef.ProxyBufferSize,
		CookieDomain:         bdef.ProxyCookieDomain,
		CookiePath:           bdef.ProxyCookiePath,
		NextUpstream:         bdef.ProxyNextUpstream,
		NextUpstreamTimeout:  bdef.ProxyNextUpstreamTimeout,
		NextUpstreamTries:    bdef.ProxyNextUpstreamTries,
		RequestBuffering:     bdef.ProxyRequestBuffering,
		ProxyRedirectFrom:    bdef.ProxyRedirectFrom,
		ProxyBuffering:       bdef.ProxyBuffering,
		ProxyHTTPVersion:     bdef.ProxyHTTPVersion,
		ProxyMaxTempFileSize: bdef.ProxyMaxTempFileSize,
	}

	// initialize default server and root location
	servers[defServerName] = &ingress.Server{
		Hostname: defServerName,
		SSLCerts: []*ingress.SSLCert{
			n.getDefaultSSLCertificate(),
		},
		Locations: []*ingress.Location{
			{
				Path:         rootLocation,
				IsDefBackend: true,
				Backend:      du.Name,
				Proxy:        ngxProxy,
				Service:      du.Service,
				Logs: log.Config{
					Access:  true,
					Rewrite: false,
				},
			},
		},
	}

	// initialize all other servers
	for _, ing := range data {
		ingKey := k8s.MetaNamespaceKey(ing)
		anns := ing.ParsedAnnotations

		// default upstream name
		un := du.Name

		if anns.Canary.Enabled {
			klog.V(2).Infof("Ingress %v is marked as Canary, ignoring", ingKey)
			continue
		}

		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			defUpstream := upstreamName(ing.Namespace, ing.Spec.DefaultBackend.Service)

			if backendUpstream, ok := upstreams[defUpstream]; ok {
				// use backend specified in Ingress as the default backend for all its rules
				un = backendUpstream.Name

				// special "catch all" case, Ingress with a backend but no rule
				defLoc := servers[defServerName].Locations[0]
				if defLoc.IsDefBackend && len(ing.Spec.Rules) == 0 {
					klog.V(2).Infof("Ingress %q defines a backend but no rule. Using it to configure the catch-all server %q",
						ingKey, defServerName)

					defLoc.IsDefBackend = false
					defLoc.Backend = backendUpstream.Name
					defLoc.Service = backendUpstream.Service
					defLoc.Ingress = ing

					// TODO: Redirect and rewrite can affect the catch all behavior, skip for now
					originalRedirect := defLoc.Redirect
					originalRewrite := defLoc.Rewrite
					locationApplyAnnotations(defLoc, anns)
					defLoc.Redirect = originalRedirect
					defLoc.Rewrite = originalRewrite
				} else {
					klog.V(3).Infof("Ingress %q defines both a backend and rules. Using its backend as default upstream for all its rules.",
						ingKey)
				}
			}
		}

		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = defServerName
			}

			if _, ok := servers[host]; ok {
				// server already configured
				continue
			}

			loc := &ingress.Location{
				Path:          rootLocation,
				IsDefBackend:  true,
				Backend:       un,
				Service:       &apiv1.Service{},
				DisableRobots: anns.DisableRobots,
			}
			locationApplyAnnotations(loc, anns)

			servers[host] = &ingress.Server{
				VirtualService: ingKey,
				Hostname:       host,
				Locations: []*ingress.Location{
					loc,
				},
				SSLPassthrough:  anns.SSLPassthrough,
				SSLCiphers:      anns.SSLCiphers,
				NeedDefaultCert: anns.DefaultCert.NeedDefault,
				SSLProtocols:    anns.SSLProtocols,
			}
		}
	}

	// configure default location, alias, and SSL
	for _, ing := range data {
		ingKey := k8s.MetaNamespaceKey(ing)
		anns := ing.ParsedAnnotations

		if anns.Canary.Enabled {
			klog.V(2).Infof("Ingress %v is marked as Canary, ignoring", ingKey)
			continue
		}

		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = defServerName
			}

			if len(servers[host].Aliases) == 0 {
				servers[host].Aliases = anns.Aliases
				if _, ok := allAliases[host]; !ok {
					allAliases[host] = anns.Aliases
				}
			} else {
				klog.Warningf("Aliases already configured for server %q, skipping (Ingress %q)", host, ingKey)
			}

			if anns.ServerSnippet != "" {
				if servers[host].ServerSnippet == "" {
					servers[host].ServerSnippet = anns.ServerSnippet
				} else {
					klog.Warningf("Server snippet already configured for server %q, skipping (Ingress %q)",
						host, ingKey)
				}
			}

			if !servers[host].NeedDefaultCert && anns.DefaultCert.NeedDefault {
				servers[host].NeedDefaultCert = anns.DefaultCert.NeedDefault
			}

			// only add SSL ciphers if the server does not have them previously configured
			if servers[host].SSLCiphers == "" && anns.SSLCiphers != "" {
				servers[host].SSLCiphers = anns.SSLCiphers
			}

			// only add certificates if the server does not have both ECC and RSA previously configured
			if len(servers[host].SSLCerts) > 1 {
				continue
			}

			if len(ing.Spec.TLS) == 0 {
				klog.V(3).Infof("Ingress %q does not contains a TLS section.", ingKey)
				continue
			}

			tlsSecretNames := extractTLSSecretName(host, ing, n.store.GetLocalSSLCert)
			if len(tlsSecretNames) == 0 {
				klog.V(3).Infof("Host %q is listed in the TLS section but secretNames are empty. Using default certificate.", host)
				servers[host].SSLCerts = append(servers[host].SSLCerts, n.getDefaultSSLCertificate())
				continue
			}

			for _, tlsSecretName := range tlsSecretNames {
				secrKey := fmt.Sprintf("%v/%v", ing.Namespace, tlsSecretName)
				cert, err := n.store.GetLocalSSLCert(secrKey)
				if err != nil {
					klog.Warningf("Error getting SSL certificate %q: %v.", secrKey, err)
					continue
				}

				err = cert.Certificate.VerifyHostname(host)
				if err != nil {
					klog.Warningf("Unexpected error validating SSL certificate %q for server %q: %v", secrKey, host, err)
					klog.Warning("Validating certificate against DNS names. This will be deprecated in a future version.")
					// check the Common Name field
					// https://github.com/golang/go/issues/22922
					err := verifyHostname(host, cert.Certificate)
					if err != nil {
						klog.Warningf("SSL certificate %q does not contain a Common Name or Subject Alternative Name for server %q: %v",
							secrKey, host, err)
						continue
					}
				}

				servers[host].SSLCerts = append(servers[host].SSLCerts, cert)

				if cert.ExpireTime.Before(time.Now().Add(240 * time.Hour)) {
					klog.Warningf("SSL certificate for server %q is about to expire (%v)", host, cert.ExpireTime)
				}
			}

			if len(servers[host].SSLCerts) == 0 {
				klog.Warningf("Using default certificate")
				servers[host].SSLCerts = append(servers[host].SSLCerts, n.getDefaultSSLCertificate())
			}
		}
	}

	for host, hostAliases := range allAliases {
		if _, ok := servers[host]; !ok {
			continue
		}

		uniqAliases := sets.NewString()
		for _, alias := range hostAliases {
			if alias == host {
				continue
			}

			if _, ok := servers[alias]; ok {
				continue
			}

			if uniqAliases.Has(alias) {
				continue
			}

			uniqAliases.Insert(alias)
		}

		servers[host].Aliases = uniqAliases.List()
	}

	return servers
}

func locationApplyAnnotations(loc *ingress.Location, anns *annotations.Ingress) {
	loc.BasicDigestAuth = anns.BasicDigestAuth
	loc.ClientBodyBufferSize = anns.ClientBodyBufferSize
	loc.ConfigurationSnippet = anns.ConfigurationSnippet
	loc.CorsConfig = anns.CorsConfig
	loc.ExternalAuth = anns.ExternalAuth
	loc.EnableGlobalAuth = anns.EnableGlobalAuth
	loc.HTTP2PushPreload = anns.HTTP2PushPreload
	loc.Opentracing = anns.Opentracing
	loc.Proxy = anns.Proxy
	loc.ProxySSL = anns.ProxySSL
	loc.RateLimit = anns.RateLimit
	loc.Redirect = anns.Redirect
	loc.Rewrite = anns.Rewrite
	loc.UpstreamVhost = anns.UpstreamVhost
	loc.Whitelist = anns.Whitelist
	loc.Denied = anns.Denied
	loc.XForwardedPrefix = anns.XForwardedPrefix
	loc.UsePortInRedirects = anns.UsePortInRedirects
	loc.Connection = anns.Connection
	loc.Logs = anns.Logs
	loc.InfluxDB = anns.InfluxDB
	loc.DefaultBackend = anns.DefaultBackend
	loc.BackendProtocol = anns.BackendProtocol
	loc.FastCGI = anns.FastCGI
	loc.CustomHTTPErrors = anns.CustomHTTPErrors
	loc.ModSecurity = anns.ModSecurity
	loc.Satisfy = anns.Satisfy
	loc.Mirror = anns.Mirror
	loc.DefaultBackendUpstreamName = defUpstreamName
	loc.LocationPreceding = anns.Location.LocationPreceding
	loc.LocationPathPrefix = anns.Location.LocationPathPrefix
	loc.LocationPathEscape = anns.Location.LocationPathEscape
}

// OK to merge canary ingresses iff there exists one or more ingresses to potentially merge into
func nonCanaryIngressExists(ingresses []*ingress.Ingress, canaryIngresses []*ingress.Ingress) bool {
	return len(ingresses) > 0 && len(canaryIngresses) > 0
}

// ensure that the following conditions are met
// 1) names of backends do not match and canary doesn't merge into itself
// 2) primary name is not the default upstream
// 3) the primary has a server
func canMergeBackend(primary *ingress.Backend, alternative *ingress.Backend) bool {
	return alternative != nil && primary.Name != alternative.Name && primary.Name != defUpstreamName && !primary.NoServer
}

// Performs the merge action and checks to ensure that one two alternative backends do not merge into each other
func mergeAlternativeBackend(priUps *ingress.Backend, altUps *ingress.Backend) bool {
	if priUps.NoServer {
		klog.Warningf("unable to merge alternative backend %v into primary backend %v because %v is a primary backend",
			altUps.Name, priUps.Name, priUps.Name)
		return false
	}

	for _, ab := range priUps.AlternativeBackends {
		if ab == altUps.Name {
			klog.V(2).Infof("skip merge alternative backend %v into %v, it's already present", altUps.Name, priUps.Name)
			return true
		}
	}

	klog.Infof("merge alternative backend %v into primary backend %v", altUps.Name, priUps.Name)
	priUps.AlternativeBackends =
		append(priUps.AlternativeBackends, altUps.Name)

	return true
}

// Compares an Ingress of a potential alternative backend's rules with each existing server and finds matching host + path pairs.
// If a match is found, we know that this server should back the alternative backend and add the alternative backend
// to a backend's alternative list.
// If no match is found, then the serverless backend is deleted.
func (n *NGINXController) mergeAlternativeBackends(ing *ingress.Ingress, upstreams map[string]*ingress.Backend,
	servers map[string]*ingress.Server) {

	// merge catch-all alternative backends
	if ing.Spec.DefaultBackend != nil {
		upsName := upstreamName(ing.Namespace, ing.Spec.DefaultBackend.Service)
		altUps := upstreams[upsName]

		if altUps == nil {
			klog.Warningf("alternative backend %s has already been removed", upsName)
		} else {

			merged := false
			altEqualsPri := false

			for _, loc := range servers[defServerName].Locations {
				priUps := upstreams[loc.Backend]
				altEqualsPri = altUps.Name == priUps.Name
				if altEqualsPri {
					klog.Warningf("alternative upstream %s in Ingress %s/%s is primary upstream in Other Ingress for location %s%s!",
						altUps.Name, ing.Namespace, ing.Name, servers[defServerName].Hostname, loc.Path)
					break
				}

				if canMergeBackend(priUps, altUps) {
					klog.V(2).Infof("matching backend %v found for alternative backend %v",
						priUps.Name, altUps.Name)

					merged = mergeAlternativeBackend(priUps, altUps)
				}
			}

			if !altEqualsPri && !merged {
				klog.Warningf("unable to find real backend for alternative backend %v. Deleting.", altUps.Name)
				delete(upstreams, altUps.Name)
			}
		}
	}

	for _, rule := range ing.Spec.Rules {
		host := rule.Host
		if host == "" {
			host = defServerName
		}

		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				// skip non-service backends
				klog.V(3).Infof("Ingress %q and path %q does not contain a service backend, using default backend", k8s.MetaNamespaceKey(ing), path.Path)
				continue
			}

			upsName := upstreamName(ing.Namespace, path.Backend.Service)

			altUps := upstreams[upsName]

			if altUps == nil {
				klog.Warningf("alternative backend %s has already been removed", upsName)
				continue
			}

			merged := false
			altEqualsPri := false

			server, ok := servers[host]
			if !ok {
				klog.Errorf("cannot merge alternative backend %s into hostname %s that does not exist",
					altUps.Name,
					host)

				continue
			}

			// find matching paths
			for _, loc := range server.Locations {
				priUps := upstreams[loc.Backend]
				if priUps == nil {
					klog.Warningf("primary backend %s has already been removed", loc.Backend)
					continue
				}
				altEqualsPri = altUps.Name == priUps.Name
				if altEqualsPri {
					klog.Warningf("alternative upstream %s in Ingress %s/%s is primary upstream in Other Ingress for location %s%s!",
						altUps.Name, ing.Namespace, ing.Name, server.Hostname, loc.Path)
					break
				}

				if canMergeBackend(priUps, altUps) && loc.Path == path.Path {
					klog.V(2).Infof("matching backend %v found for alternative backend %v",
						priUps.Name, altUps.Name)
					merged = mergeAlternativeBackend(priUps, altUps)
					if merged {
						canary := &ingress.Canary{
							Target:               upsName,
							TrafficShapingPolicy: altUps.TrafficShapingPolicy,
						}
						klog.Infof("append alternative upstream %s in Ingress %s/%s to %s%s", altUps.Name, ing.Namespace, ing.Name, server.Hostname, loc.Path)
						loc.Canaries = append(loc.Canaries, canary)
					}
				}
			}

			if !altEqualsPri && !merged {
				klog.Warningf("unable to find real backend for alternative backend %v. Deleting.", altUps.Name)
				delete(upstreams, altUps.Name)
			}
		}
	}
}

// extractTLSSecretName returns the name of the Secret containing a SSL
// certificate for the given host name, or an empty string.
func extractTLSSecretName(host string, ing *ingress.Ingress,
	getLocalSSLCert func(string) (*ingress.SSLCert, error)) []string {
	if ing == nil {
		return nil
	}

	// naively return Secret name from TLS spec if host name matches
	secretNames := []string{}
	for _, tls := range ing.Spec.TLS {
		if sets.NewString(tls.Hosts...).Has(host) {
			klog.Infof("Found a secret %v containing a SSL certificate for the given host name %q", tls.SecretName, host)
			secretNames = append(secretNames, tls.SecretName)
		}
	}
	if len(secretNames) > 0 {
		return secretNames
	}

	// no TLS host matching host name, try each TLS host for matching SAN or CN
	for _, tls := range ing.Spec.TLS {

		if tls.SecretName == "" {
			// There's no secretName specified, so it will never be available
			continue
		}

		secrKey := fmt.Sprintf("%v/%v", ing.Namespace, tls.SecretName)

		cert, err := getLocalSSLCert(secrKey)
		if err != nil {
			klog.Warningf("Error getting SSL certificate %q: %v", secrKey, err)
			continue
		}

		if cert == nil { // for tests
			continue
		}

		err = cert.Certificate.VerifyHostname(host)
		if err != nil {
			continue
		}
		klog.V(3).Infof("Found SSL certificate %v matching host %q: %q", tls.SecretName, host, secrKey)
		secretNames = append(secretNames, tls.SecretName)
	}

	return secretNames
}

// getRemovedHosts returns a list of the hostsnames
// that are not associated anymore to the NGINX configuration.
func getRemovedHosts(rucfg, newcfg *ingress.Configuration) []string {
	old := sets.NewString()
	new := sets.NewString()

	for _, s := range rucfg.Servers {
		if !old.Has(s.Hostname) {
			old.Insert(s.Hostname)
		}
	}

	for _, s := range newcfg.Servers {
		if !new.Has(s.Hostname) {
			new.Insert(s.Hostname)
		}
	}

	return old.Difference(new).List()
}

func getRemovedIngresses(rucfg, newcfg *ingress.Configuration) []string {
	oldIngresses := sets.NewString()
	newIngresses := sets.NewString()

	for _, server := range rucfg.Servers {
		for _, location := range server.Locations {
			if location.Ingress == nil {
				continue
			}

			ingKey := k8s.MetaNamespaceKey(location.Ingress)
			if !oldIngresses.Has(ingKey) {
				oldIngresses.Insert(ingKey)
			}
		}
	}

	for _, server := range newcfg.Servers {
		for _, location := range server.Locations {
			if location.Ingress == nil {
				continue
			}

			ingKey := k8s.MetaNamespaceKey(location.Ingress)
			if !newIngresses.Has(ingKey) {
				newIngresses.Insert(ingKey)
			}
		}
	}

	return oldIngresses.Difference(newIngresses).List()
}

// checks conditions for whether or not an upstream should be created for a custom default backend
func shouldCreateUpstreamForLocationDefaultBackend(upstream *ingress.Backend, location *ingress.Location) bool {
	return (upstream.Name == location.Backend) &&
		(len(upstream.Endpoints) == 0 || len(location.CustomHTTPErrors) != 0) &&
		location.DefaultBackend != nil
}

func externalNamePorts(name string, svc *apiv1.Service) *apiv1.ServicePort {
	port, err := strconv.Atoi(name)
	if err != nil {
		// not a number. check port names.
		for _, svcPort := range svc.Spec.Ports {
			if svcPort.Name != name {
				continue
			}

			tp := svcPort.TargetPort
			if tp.IntValue() == 0 {
				tp = intstr.FromInt(int(svcPort.Port))
			}

			return &apiv1.ServicePort{
				Protocol:   "TCP",
				Port:       svcPort.Port,
				TargetPort: tp,
			}
		}
	}

	for _, svcPort := range svc.Spec.Ports {
		if svcPort.Port != int32(port) {
			continue
		}

		tp := svcPort.TargetPort
		if tp.IntValue() == 0 {
			tp = intstr.FromInt(port)
		}

		return &apiv1.ServicePort{
			Protocol:   "TCP",
			Port:       svcPort.Port,
			TargetPort: svcPort.TargetPort,
		}
	}

	// ExternalName without port
	return &apiv1.ServicePort{
		Protocol:   "TCP",
		Port:       int32(port),
		TargetPort: intstr.FromInt(port),
	}
}

func (n *NGINXController) verifyCanaryReferrer(ingKey string, anns *annotations.Ingress) bool {
	if anns.Canary.Referrer == "" {
		klog.Infof("Canary ingress[%v] with empty referrer", ingKey)
		return true
	}

	cfg := n.store.GetBackendConfiguration()
	canaryReferrers := strings.Split(cfg.CanaryReferrer, ",")
	for _, canaryReferrer := range canaryReferrers {
		if canaryReferrer == anns.Canary.Referrer {
			return true
		}
	}

	n.metricCollector.IncCanaryReferInvalidCount()
	klog.Warningf("Canary ingress[%v] with referrer [%v] is illegal, ignored", ingKey, anns.Canary.Referrer)
	return false
}
