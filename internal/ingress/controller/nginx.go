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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	proxyproto "github.com/armon/go-proxyproto"
	"github.com/eapache/channels"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog"

	adm_controller "k8s.io/ingress-nginx/internal/admission/controller"
	"k8s.io/ingress-nginx/internal/file"
	"k8s.io/ingress-nginx/internal/ingress"
	"k8s.io/ingress-nginx/internal/ingress/annotations/class"
	ngx_config "k8s.io/ingress-nginx/internal/ingress/controller/config"
	"k8s.io/ingress-nginx/internal/ingress/controller/process"
	"k8s.io/ingress-nginx/internal/ingress/controller/store"
	ngx_template "k8s.io/ingress-nginx/internal/ingress/controller/template"
	"k8s.io/ingress-nginx/internal/ingress/metric"
	"k8s.io/ingress-nginx/internal/ingress/status"
	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/internal/lock"
	ing_net "k8s.io/ingress-nginx/internal/net"
	"k8s.io/ingress-nginx/internal/net/dns"
	"k8s.io/ingress-nginx/internal/net/ssl"
	"k8s.io/ingress-nginx/internal/nginx"
	"k8s.io/ingress-nginx/internal/task"
	"k8s.io/ingress-nginx/internal/watch"
)

const (
	tempNginxPattern = "nginx-cfg"
	emptyUID         = "-1"
)

// NewNGINXController creates a new Tengine Ingress controller.
func NewNGINXController(config *Configuration, mc metric.Collector) *NGINXController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: config.Client.CoreV1().Events(config.Namespace),
	})

	h, err := dns.GetSystemNameServers()
	if err != nil {
		klog.Warningf("Error reading system nameservers: %v", err)
	}

	n := &NGINXController{
		isIPV6Enabled: ing_net.IsIPv6Enabled(),

		resolver:        h,
		cfg:             config,
		syncRateLimiter: flowcontrol.NewTokenBucketRateLimiter(config.SyncRateLimit, 1),

		recorder: eventBroadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{
			Component: "tengine-ingress-controller",
		}),

		stopCh:   make(chan struct{}),
		updateCh: channels.NewRingChannel(1024),

		ngxErrCh: make(chan error),

		stopLock: &sync.Mutex{},

		runningConfig: new(ingress.Configuration),

		Proxy: &TCPProxy{},

		metricCollector: mc,

		command: NewNginxCommand(),

		checksumStatus: new(ingress.ChecksumStatus),
	}

	if n.cfg.ValidationWebhook != "" {
		n.validationWebhookServer = &http.Server{
			Addr:      config.ValidationWebhook,
			Handler:   adm_controller.NewAdmissionControllerServer(&adm_controller.IngressAdmission{Checker: n}),
			TLSConfig: ssl.NewTLSListener(n.cfg.ValidationWebhookCertPath, n.cfg.ValidationWebhookKeyPath).TLSConfig(),
		}
	}

	if err != k8s.GetIngressPod(config.Client) {
		klog.Fatalf("unexpected error obtaining pod information: %v", err)
	}
	n.podInfo = k8s.IngressPodDetails

	n.store = store.New(
		config.Namespace,
		config.ConfigMapName,
		config.TCPConfigMapName,
		config.UDPConfigMapName,
		config.DefaultSSLCertificate,
		config.ResyncPeriod,
		config.Client,
		config.ClientIng,
		config.ClientIngCheck,
		config.ClientSecretCheck,
		n.metricCollector,
		n.updateCh,
		k8s.IngressPodDetails,
		config.DisableCatchAll,
		n.checksumStatus)

	n.syncQueue = task.NewTaskQueue(n.syncIngress)

	if config.UpdateStatus {
		n.syncStatus = status.NewStatusSyncer(status.Config{
			Client:                 config.Client,
			ClientIng:              config.ClientIng,
			PublishService:         config.PublishService,
			PublishStatusAddress:   config.PublishStatusAddress,
			IngressLister:          n.store,
			UpdateStatusOnShutdown: config.UpdateStatusOnShutdown,
			UseNodeInternalIP:      config.UseNodeInternalIP,
		})
	} else {
		klog.Warning("Update of Ingress status is disabled (flag --update-status)")
	}

	onTemplateChange := func() {
		template, err := ngx_template.NewTemplate(nginx.TemplatePath)
		if err != nil {
			// this error is different from the rest because it must be clear why nginx is not working
			klog.Errorf(`
-------------------------------------------------------------------------------
Error loading new template: %v
-------------------------------------------------------------------------------
`, err)
			return
		}

		n.t = template
		klog.Info("New Tengine configuration template loaded.")
		n.syncQueue.EnqueueTask(task.GetDummyObject("template-change"))
	}

	ngxTpl, err := ngx_template.NewTemplate(nginx.TemplatePath)
	if err != nil {
		klog.Fatalf("Invalid Tengine configuration template: %v", err)
	}

	n.t = ngxTpl

	_, err = watch.NewFileWatcher(nginx.TemplatePath, onTemplateChange)
	if err != nil {
		klog.Fatalf("Error creating file watcher for %v: %v", nginx.TemplatePath, err)
	}

	filesToWatch := []string{}
	err = filepath.Walk("/etc/nginx/geoip/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		filesToWatch = append(filesToWatch, path)
		return nil
	})

	if err != nil {
		klog.Fatalf("Error creating file watchers: %v", err)
	}

	for _, f := range filesToWatch {
		_, err = watch.NewFileWatcher(f, func() {
			klog.Infof("File %v changed. Reloading Tengine", f)
			n.syncQueue.EnqueueTask(task.GetDummyObject("file-change"))
		})
		if err != nil {
			klog.Fatalf("Error creating file watcher for %v: %v", f, err)
		}
	}

	return n
}

// NGINXController describes a Tengine Ingress controller.
type NGINXController struct {
	podInfo *k8s.PodInfo

	cfg *Configuration

	recorder record.EventRecorder

	syncQueue *task.Queue

	syncStatus status.Syncer

	syncRateLimiter flowcontrol.RateLimiter

	// stopLock is used to enforce that only a single call to Stop send at
	// a given time. We allow stopping through an HTTP endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock *sync.Mutex

	stopCh   chan struct{}
	updateCh *channels.RingChannel

	// ngxErrCh is used to detect errors with the Tengine processes
	ngxErrCh chan error

	// runningConfig contains the running configuration in the Backend
	runningConfig *ingress.Configuration

	t ngx_template.TemplateWriter

	resolver []net.IP

	isIPV6Enabled bool

	isShuttingDown bool

	Proxy *TCPProxy

	store store.Storer

	metricCollector metric.Collector

	validationWebhookServer *http.Server

	command NginxExecTester

	isInitReLoad bool

	isInitLoadCfg bool

	checksumStatus *ingress.ChecksumStatus

	hotReloadMD5 string
}

// Start starts a new Tengine master process running in the foreground.
func (n *NGINXController) Start() {
	klog.Info("Starting Tengine Ingress controller")

	n.store.Run(n.stopCh)

	// we need to use the defined ingress class to allow multiple leaders
	// in order to update information about ingress status
	electionID := fmt.Sprintf("%v-%v", n.cfg.ElectionID, class.DefaultClass)
	if class.IngressClass != "" {
		electionID = fmt.Sprintf("%v-%v", n.cfg.ElectionID, class.IngressClass)
	}

	setupLeaderElection(&leaderElectionConfig{
		Client:     n.cfg.Client,
		ElectionID: electionID,
		OnStartedLeading: func(stopCh chan struct{}) {
			if n.syncStatus != nil {
				go n.syncStatus.Run(stopCh)
			}

			n.metricCollector.OnStartedLeading(electionID)
			// manually update SSL expiration metrics
			// (to not wait for a reload)
			n.metricCollector.SetSSLExpireTime(n.runningConfig.Servers)
		},
		OnStoppedLeading: func() {
			n.metricCollector.OnStoppedLeading(electionID)
		},
		PodName:      n.podInfo.Name,
		PodNamespace: n.podInfo.Namespace,
	})

	if !n.isInitLoadCfg {
		klog.Info("Init hot reloading cfg")
		ngxCfg := n.store.GetBackendConfiguration()
		var ingCfg ingress.Configuration
		md5, err := hotReload(n.hotReloadMD5, ngxCfg, ingCfg, true)
		if err != nil {
			klog.Fatalf("Init hot reloading cfg failed:\n%v", err)
		}
		n.isInitLoadCfg = true
		n.hotReloadMD5 = md5
	}

	cmd := n.command.ExecCommand()

	// put Tengine in another process group to prevent it
	// to receive signals meant for the controller
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if n.cfg.EnableSSLPassthrough {
		n.setupSSLProxy()
	}

	klog.Info("Starting Tengine process")
	n.start(cmd)

	go n.syncQueue.Run(time.Second, n.stopCh)
	// force initial sync
	n.syncQueue.EnqueueTask(task.GetDummyObject("initial-sync"))

	// In case of error the temporal configuration file will
	// be available up to five minutes after the error
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			err := cleanTempNginxCfg()
			if err != nil {
				klog.Infof("Unexpected error removing temporal configuration files: %v", err)
			}
		}
	}()

	if n.validationWebhookServer != nil {
		klog.Infof("Starting validation webhook on %s with keys %s %s", n.validationWebhookServer.Addr, n.cfg.ValidationWebhookCertPath, n.cfg.ValidationWebhookKeyPath)
		go func() {
			klog.Error(n.validationWebhookServer.ListenAndServeTLS("", ""))
		}()
	}

	for {
		select {
		case err := <-n.ngxErrCh:
			if n.isShuttingDown {
				return
			}

			// if the nginx master process dies the workers continue to process requests,
			// passing checks but in case of updates in ingress no updates will be
			// reflected in the nginx configuration which can lead to confusion and report
			// issues because of this behavior.
			// To avoid this issue we restart nginx in case of errors.
			if process.IsRespawnIfRequired(err) {
				process.WaitUntilPortIsAvailable(n.cfg.ListenPorts.HTTP)
				// release command resources
				cmd.Process.Release()
				// start a new nginx master process if the controller is not being stopped
				cmd = n.command.ExecCommand()
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Setpgid: true,
					Pgid:    0,
				}
				n.start(cmd)
			}
		case event := <-n.updateCh.Out():
			if n.isShuttingDown {
				break
			}

			if evt, ok := event.(store.Event); ok {
				klog.V(3).Infof("Event %v received - object %v", evt.Type, evt.Obj)

				if evt.Type == store.ConfigurationEvent {
					// TODO: is this necessary? Consider removing this special case
					n.syncQueue.EnqueueTask(task.GetDummyObject("configmap-change"))
					continue
				}

				n.syncQueue.EnqueueSkippableTask(evt.Obj)
			} else {
				klog.Warningf("Unexpected event type received %T", event)
			}
		case <-n.stopCh:
			return
		}
	}
}

// Stop gracefully stops the Tengine master process.
func (n *NGINXController) Stop() error {
	n.isShuttingDown = true

	n.stopLock.Lock()
	defer n.stopLock.Unlock()

	if n.syncQueue.IsShuttingDown() {
		return fmt.Errorf("shutdown already in progress")
	}

	klog.Info("Shutting down controller queues")
	close(n.stopCh)
	go n.syncQueue.Shutdown()
	if n.syncStatus != nil {
		n.syncStatus.Shutdown()
	}

	if n.validationWebhookServer != nil {
		klog.Info("Stopping admission controller")
		err := n.validationWebhookServer.Close()
		if err != nil {
			return err
		}
	}

	// send stop signal to Tengine
	klog.Info("Stopping Tengine process")
	cmd := n.command.ExecCommand("-s", "quit")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	// wait for the Tengine process to terminate
	timer := time.NewTicker(time.Second * 1)
	for range timer.C {
		if !nginx.IsRunning() {
			klog.Info("Tengine process has stopped")
			timer.Stop()
			break
		}
	}

	return nil
}

func (n *NGINXController) start(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		klog.Fatalf("Tengine error: %v", err)
		n.ngxErrCh <- err
		return
	}

	go func() {
		n.ngxErrCh <- cmd.Wait()
	}()
}

// DefaultEndpoint returns the default endpoint to be use as default server that returns 404.
func (n NGINXController) DefaultEndpoint() ingress.Endpoint {
	return ingress.Endpoint{
		Address: "127.0.0.1",
		Port:    fmt.Sprintf("%v", n.cfg.ListenPorts.Default),
		Target:  &apiv1.ObjectReference{},
	}
}

// generateTemplate returns the nginx configuration file content
func (n NGINXController) generateTemplate(cfg ngx_config.Configuration, ingressCfg ingress.Configuration) ([]byte, error) {
	ingServers := []*ingress.Server{}
	if !cfg.TengineStaticServiceCfg {
		for _, srv := range ingressCfg.Servers {
			if srv.Hostname == "_" {
				ingServers = append(ingServers, srv)
				break
			}
		}
	}

	if n.cfg.EnableSSLPassthrough {
		servers := []*TCPServer{}
		for _, pb := range ingressCfg.PassthroughBackends {
			svc := pb.Service
			if svc == nil {
				klog.Warningf("Missing Service for SSL Passthrough backend %q", pb.Backend)
				continue
			}
			port, err := strconv.Atoi(pb.Port.String())
			if err != nil {
				for _, sp := range svc.Spec.Ports {
					if sp.Name == pb.Port.String() {
						port = int(sp.Port)
						break
					}
				}
			} else {
				for _, sp := range svc.Spec.Ports {
					if sp.Port == int32(port) {
						port = int(sp.Port)
						break
					}
				}
			}

			// TODO: Allow PassthroughBackends to specify they support proxy-protocol
			servers = append(servers, &TCPServer{
				Hostname:      pb.Hostname,
				IP:            svc.Spec.ClusterIP,
				Port:          port,
				ProxyProtocol: false,
			})
		}

		n.Proxy.ServerList = servers
	}

	// Tengine cannot resize the hash tables used to store server names. For
	// this reason we check if the current size is correct for the host
	// names defined in the Ingress rules and adjust the value if
	// necessary.
	// https://trac.nginx.org/nginx/ticket/352
	// https://trac.nginx.org/nginx/ticket/631
	var longestName int
	var serverNameBytes int

	for _, srv := range ingressCfg.Servers {
		hostnameLength := len(srv.Hostname)
		if srv.RedirectFromToWWW {
			hostnameLength += 4
		}
		if longestName < hostnameLength {
			longestName = hostnameLength
		}

		for _, alias := range srv.Aliases {
			if longestName < len(alias) {
				longestName = len(alias)
			}
		}

		serverNameBytes += hostnameLength
	}

	nameHashBucketSize := nginxHashBucketSize(longestName)
	if cfg.ServerNameHashBucketSize < nameHashBucketSize {
		klog.V(3).Infof("Adjusting ServerNameHashBucketSize variable to %d", nameHashBucketSize)
		cfg.ServerNameHashBucketSize = nameHashBucketSize
	}

	serverNameHashMaxSize := nextPowerOf2(serverNameBytes)
	if cfg.ServerNameHashMaxSize < serverNameHashMaxSize {
		klog.V(3).Infof("Adjusting ServerNameHashMaxSize variable to %d", serverNameHashMaxSize)
		cfg.ServerNameHashMaxSize = serverNameHashMaxSize
	}

	if cfg.MaxWorkerOpenFiles == 0 {
		// the limit of open files is per worker process
		// and we leave some room to avoid consuming all the FDs available
		wp, err := strconv.Atoi(cfg.WorkerProcesses)
		klog.V(3).Infof("Number of worker processes: %d", wp)
		if err != nil {
			wp = 1
		}
		maxOpenFiles := (rlimitMaxNumFiles() / wp) - 1024
		klog.V(3).Infof("Maximum number of open file descriptors: %d", maxOpenFiles)
		if maxOpenFiles < 1024 {
			// this means the value of RLIMIT_NOFILE is too low.
			maxOpenFiles = 1024
		}
		klog.V(3).Infof("Adjusting MaxWorkerOpenFiles variable to %d", maxOpenFiles)
		cfg.MaxWorkerOpenFiles = maxOpenFiles
	}

	if cfg.MaxWorkerConnections == 0 {
		maxWorkerConnections := int(float64(cfg.MaxWorkerOpenFiles * 3.0 / 4))
		klog.V(3).Infof("Adjusting MaxWorkerConnections variable to %d", maxWorkerConnections)
		cfg.MaxWorkerConnections = maxWorkerConnections
	}

	setHeaders := map[string]string{}
	if cfg.ProxySetHeaders != "" {
		cmap, err := n.store.GetConfigMap(cfg.ProxySetHeaders)
		if err != nil {
			klog.Warningf("Error reading ConfigMap %q from local store: %v", cfg.ProxySetHeaders, err)
		} else {
			setHeaders = cmap.Data
		}
	}

	addHeaders := map[string]string{}
	if cfg.AddHeaders != "" {
		cmap, err := n.store.GetConfigMap(cfg.AddHeaders)
		if err != nil {
			klog.Warningf("Error reading ConfigMap %q from local store: %v", cfg.AddHeaders, err)
		} else {
			addHeaders = cmap.Data
		}
	}

	sslDHParam := ""
	if cfg.SSLDHParam != "" {
		secretName := cfg.SSLDHParam

		secret, err := n.store.GetSecret(secretName)
		if err != nil {
			klog.Warningf("Error reading Secret %q from local store: %v", secretName, err)
		} else {
			nsSecName := strings.Replace(secretName, "/", "-", -1)
			dh, ok := secret.Data["dhparam.pem"]
			if ok {
				pemFileName, err := ssl.AddOrUpdateDHParam(nsSecName, dh)
				if err != nil {
					klog.Warningf("Error adding or updating dhparam file %v: %v", nsSecName, err)
				} else {
					sslDHParam = pemFileName
				}
			}
		}
	}

	cfg.SSLDHParam = sslDHParam
	cfg.DefaultSSLCertificate = n.getDefaultSSLCertificate()

	var tc ngx_config.TemplateConfig
	if !cfg.TengineStaticServiceCfg {
		klog.Infof("Ignore tengine static configurations")
		tc = ngx_config.TemplateConfig{
			ProxySetHeaders:          setHeaders,
			AddHeaders:               addHeaders,
			BacklogSize:              sysctlSomaxconn(),
			Backends:                 ingressCfg.Backends,
			PassthroughBackends:      ingressCfg.PassthroughBackends,
			Servers:                  ingServers,
			TCPBackends:              ingressCfg.TCPEndpoints,
			UDPBackends:              ingressCfg.UDPEndpoints,
			Cfg:                      cfg,
			IsIPV6Enabled:            n.isIPV6Enabled && !cfg.DisableIpv6,
			NginxStatusIpv4Whitelist: cfg.NginxStatusIpv4Whitelist,
			NginxStatusIpv6Whitelist: cfg.NginxStatusIpv6Whitelist,
			RedirectServers:          buildRedirects(ingServers),
			IsSSLPassthroughEnabled:  n.cfg.EnableSSLPassthrough,
			ListenPorts:              n.cfg.ListenPorts,
			PublishService:           n.GetPublishService(),
			EnableMetrics:            n.cfg.EnableMetrics,

			HealthzURI: nginx.HealthPath,
			PID:        nginx.PID,
			StatusPath: nginx.StatusPath,
			StatusPort: nginx.StatusPort,
			StreamPort: nginx.StreamPort,

			DefaultServers: buildDefaultServers(ingServers, cfg.DefaultCertPorts),
		}
	} else {
		tc = ngx_config.TemplateConfig{
			ProxySetHeaders:          setHeaders,
			AddHeaders:               addHeaders,
			BacklogSize:              sysctlSomaxconn(),
			Backends:                 ingressCfg.Backends,
			PassthroughBackends:      ingressCfg.PassthroughBackends,
			Servers:                  ingressCfg.Servers,
			TCPBackends:              ingressCfg.TCPEndpoints,
			UDPBackends:              ingressCfg.UDPEndpoints,
			Cfg:                      cfg,
			IsIPV6Enabled:            n.isIPV6Enabled && !cfg.DisableIpv6,
			NginxStatusIpv4Whitelist: cfg.NginxStatusIpv4Whitelist,
			NginxStatusIpv6Whitelist: cfg.NginxStatusIpv6Whitelist,
			RedirectServers:          buildRedirects(ingressCfg.Servers),
			IsSSLPassthroughEnabled:  n.cfg.EnableSSLPassthrough,
			ListenPorts:              n.cfg.ListenPorts,
			PublishService:           n.GetPublishService(),
			EnableMetrics:            n.cfg.EnableMetrics,

			HealthzURI: nginx.HealthPath,
			PID:        nginx.PID,
			StatusPath: nginx.StatusPath,
			StatusPort: nginx.StatusPort,
			StreamPort: nginx.StreamPort,

			DefaultServers: buildDefaultServers(ingressCfg.Servers, cfg.DefaultCertPorts),
		}
	}

	tc.Cfg.Checksum = ingressCfg.ConfigurationChecksum

	return n.t.Write(tc)
}

// testTemplate checks if the Tengine configuration inside the byte array is valid
// running the command "nginx -t" using a temporal file.
func (n NGINXController) testTemplate(cfg []byte) error {
	if len(cfg) == 0 {
		return fmt.Errorf("invalid Tengine configuration (empty)")
	}
	tmpfile, err := ioutil.TempFile("", tempNginxPattern)
	if err != nil {
		return err
	}
	defer tmpfile.Close()
	err = ioutil.WriteFile(tmpfile.Name(), cfg, file.ReadWriteByUser)
	if err != nil {
		return err
	}
	out, err := n.command.Test(tmpfile.Name())
	if err != nil {
		// this error is different from the rest because it must be clear why nginx is not working
		oe := fmt.Sprintf(`
-------------------------------------------------------------------------------
Error: %v
%v
-------------------------------------------------------------------------------
`, err, string(out))

		return errors.New(oe)
	}

	os.Remove(tmpfile.Name())
	return nil
}

// OnUpdate is called by the synchronization loop whenever configuration
// changes were detected. The received backend Configuration is merged with the
// configuration ConfigMap before generating the final configuration file.
// Returns nil in case the backend was successfully reloaded.
func (n *NGINXController) OnUpdate(ingressCfg ingress.Configuration) error {
	cfg := n.store.GetBackendConfiguration()
	cfg.Resolver = n.resolver

	content, err := n.generateTemplate(cfg, ingressCfg)
	if err != nil {
		return err
	}

	err = createOpentracingCfg(cfg)
	if err != nil {
		return err
	}

	err = n.testTemplate(content)
	if err != nil {
		return err
	}

	if klog.V(2) {
		src, _ := ioutil.ReadFile(cfgPath)
		if !bytes.Equal(src, content) {
			tmpfile, err := ioutil.TempFile("", "new-nginx-cfg")
			if err != nil {
				return err
			}
			defer tmpfile.Close()
			err = ioutil.WriteFile(tmpfile.Name(), content, file.ReadWriteByUser)
			if err != nil {
				return err
			}

			diffOutput, err := exec.Command("diff", "-I", "'# Configuration.*'", "-u", cfgPath, tmpfile.Name()).CombinedOutput()
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					ws := exitError.Sys().(syscall.WaitStatus)
					if ws.ExitStatus() == 2 {
						klog.Warningf("Failed to executing diff command: %v", err)
					}
				}
			}

			klog.Infof("Tengine configuration diff:\n%v", string(diffOutput))

			// we do not defer the deletion of temp files in order
			// to keep them around for inspection in case of error
			os.Remove(tmpfile.Name())
		}
	}

	err = ioutil.WriteFile(cfgPath, content, file.ReadWriteByUser)
	if err != nil {
		return err
	}

	if !cfg.TengineReload && n.isInitReLoad {
		klog.Infof("Ignore tengine reload {TengineReload[%v], InitReLoad[%v]}", cfg.TengineReload, n.isInitReLoad)
		return nil
	}

	klog.Infof("Reloading Tengine work processes [%v]", cfg.WorkerProcesses)

	o, err := n.command.ExecCommand("-s", "reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%v", err, string(o))
	}

	n.isInitReLoad = true
	// wait for the tengine new worker processes to start
	timer := time.NewTicker(time.Second * 1)
	for range timer.C {
		if runtime.NumCPU() <= nginx.WorkerProcessesNum() {
			lock.RemoveFile(cfg.StatusTengineFilePath)
			klog.Infof("Tengine reload successfully")
			break
		}
	}

	return nil
}

// nginxHashBucketSize computes the correct Tengine hash_bucket_size for a hash
// with the given longest key.
func nginxHashBucketSize(longestString int) int {
	// see https://github.com/kubernetes/ingress-nginxs/issues/623 for an explanation
	wordSize := 8 // Assume 64 bit CPU
	n := longestString + 2
	aligned := (n + wordSize - 1) & ^(wordSize - 1)
	rawSize := wordSize + wordSize + aligned
	return nextPowerOf2(rawSize)
}

// http://graphics.stanford.edu/~seander/bithacks.html#RoundUpPowerOf2
// https://play.golang.org/p/TVSyCcdxUh
func nextPowerOf2(v int) int {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++

	return v
}

func (n *NGINXController) setupSSLProxy() {
	cfg := n.store.GetBackendConfiguration()
	sslPort := n.cfg.ListenPorts.HTTPS
	proxyPort := n.cfg.ListenPorts.SSLProxy

	klog.Info("Starting TLS proxy for SSL Passthrough")
	n.Proxy = &TCPProxy{
		Default: &TCPServer{
			Hostname:      "localhost",
			IP:            "127.0.0.1",
			Port:          proxyPort,
			ProxyProtocol: true,
		},
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", sslPort))
	if err != nil {
		klog.Fatalf("%v", err)
	}

	proxyList := &proxyproto.Listener{Listener: listener, ProxyHeaderTimeout: cfg.ProxyProtocolHeaderTimeout}

	// accept TCP connections on the configured HTTPS port
	go func() {
		for {
			var conn net.Conn
			var err error

			if n.store.GetBackendConfiguration().UseProxyProtocol {
				// wrap the listener in order to decode Proxy
				// Protocol before handling the connection
				conn, err = proxyList.Accept()
			} else {
				conn, err = listener.Accept()
			}

			if err != nil {
				klog.Warningf("Error accepting TCP connection: %v", err)
				continue
			}

			klog.V(3).Infof("Handling connection from remote address %s to local %s", conn.RemoteAddr(), conn.LocalAddr())
			go n.Proxy.Handle(conn)
		}
	}()
}

// Helper function to clear Certificates from the ingress configuration since they should be ignored when
// checking if the new configuration changes can be applied dynamically if dynamic certificates is on
func clearCertificates(config *ingress.Configuration) {
	var clearedServers []*ingress.Server
	for _, server := range config.Servers {
		copyOfServer := *server
		copyOfServer.SSLCerts = nil
		clearedServers = append(clearedServers, &copyOfServer)
	}
	config.Servers = clearedServers
}

// Helper function to clear endpoints from the ingress configuration since they should be ignored when
// checking if the new configuration changes can be applied dynamically.
func clearL4serviceEndpoints(config *ingress.Configuration) {
	var clearedTCPL4Services []ingress.L4Service
	var clearedUDPL4Services []ingress.L4Service
	for _, service := range config.TCPEndpoints {
		copyofService := ingress.L4Service{
			Port:      service.Port,
			Backend:   service.Backend,
			Endpoints: []ingress.Endpoint{},
			Service:   nil,
		}
		clearedTCPL4Services = append(clearedTCPL4Services, copyofService)
	}
	for _, service := range config.UDPEndpoints {
		copyofService := ingress.L4Service{
			Port:      service.Port,
			Backend:   service.Backend,
			Endpoints: []ingress.Endpoint{},
			Service:   nil,
		}
		clearedUDPL4Services = append(clearedUDPL4Services, copyofService)
	}
	config.TCPEndpoints = clearedTCPL4Services
	config.UDPEndpoints = clearedUDPL4Services
}

// IsDynamicConfigurationEnough returns whether a Configuration can be
// dynamically applied, without reloading the backend.
func (n *NGINXController) IsDynamicConfigurationEnough(pcfg *ingress.Configuration) bool {
	copyOfRunningConfig := *n.runningConfig
	copyOfPcfg := *pcfg

	copyOfRunningConfig.Backends = []*ingress.Backend{}
	copyOfPcfg.Backends = []*ingress.Backend{}

	clearL4serviceEndpoints(&copyOfRunningConfig)
	clearL4serviceEndpoints(&copyOfPcfg)

	copyOfRunningConfig.ControllerPodsCount = 0
	copyOfPcfg.ControllerPodsCount = 0

	clearCertificates(&copyOfRunningConfig)
	clearCertificates(&copyOfPcfg)

	isDynamicConfigurationEnough := copyOfRunningConfig.Equal(&copyOfPcfg)
	klog.Infof("Configuration can be dynamically applied: [%v]", !isDynamicConfigurationEnough)
	return isDynamicConfigurationEnough
}

// configureDynamically encodes new Backends in JSON format and POSTs the
// payload to an internal HTTP endpoint handled by Lua.
func (n *NGINXController) configureDynamically(pcfg *ingress.Configuration) error {
	backendsChanged := !reflect.DeepEqual(n.runningConfig.Backends, pcfg.Backends)
	if backendsChanged {
		err := configureBackends(pcfg.Backends)
		if err != nil {
			return err
		}
	}

	streamConfigurationChanged := !reflect.DeepEqual(n.runningConfig.TCPEndpoints, pcfg.TCPEndpoints) || !reflect.DeepEqual(n.runningConfig.UDPEndpoints, pcfg.UDPEndpoints)
	if streamConfigurationChanged {
		err := updateStreamConfiguration(pcfg.TCPEndpoints, pcfg.UDPEndpoints)
		if err != nil {
			return err
		}
	}

	if n.runningConfig.ControllerPodsCount != pcfg.ControllerPodsCount {
		statusCode, _, err := nginx.NewPostStatusRequest("/configuration/general", "application/json", ingress.GeneralConfig{
			ControllerPodsCount: pcfg.ControllerPodsCount,
		})
		if err != nil {
			return err
		}
		if statusCode != http.StatusCreated {
			return fmt.Errorf("unexpected error code: %d", statusCode)
		}
	}

	serversChanged := !reflect.DeepEqual(n.runningConfig.Servers, pcfg.Servers)
	if serversChanged {
		if n.store == nil {
			return nil
		}

		ready, err0 := secretCheck(n.store.ListSecretsWithAnnotation(), n.store.ListLocalSecretCheckSums(nil))
		if ready {
			n.checksumStatus.SecretChecksumStatus = true
			n.metricCollector.IncSecretChecksumCount()
			n.metricCollector.ClearSecretChecksumErrorCount()
			err := configureCertificates(pcfg.Servers)
			if err != nil {
				return err
			}
		} else if err0 != nil {
			n.checksumStatus.SecretChecksumStatus = false
			klog.Errorf("Secret ID mismatch, alarm:\n\n%v", err0)
			n.metricCollector.IncSecretChecksumErrorCount()
			return err0
		}
	}

	return nil
}

func updateStreamConfiguration(TCPEndpoints []ingress.L4Service, UDPEndpoints []ingress.L4Service) error {
	streams := make([]ingress.Backend, 0)
	for _, ep := range TCPEndpoints {
		var service *apiv1.Service
		if ep.Service != nil {
			service = &apiv1.Service{Spec: ep.Service.Spec}
		}

		key := fmt.Sprintf("tcp-%v-%v-%v", ep.Backend.Namespace, ep.Backend.Name, ep.Backend.Port.String())
		streams = append(streams, ingress.Backend{
			Name:      key,
			Endpoints: ep.Endpoints,
			Port:      intstr.FromInt(ep.Port),
			Service:   service,
		})
	}
	for _, ep := range UDPEndpoints {
		var service *apiv1.Service
		if ep.Service != nil {
			service = &apiv1.Service{Spec: ep.Service.Spec}
		}

		key := fmt.Sprintf("udp-%v-%v-%v", ep.Backend.Namespace, ep.Backend.Name, ep.Backend.Port.String())
		streams = append(streams, ingress.Backend{
			Name:      key,
			Endpoints: ep.Endpoints,
			Port:      intstr.FromInt(ep.Port),
			Service:   service,
		})
	}

	buf, err := json.Marshal(streams)
	if err != nil {
		return err
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%v", nginx.StreamPort))
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(buf)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(conn, "\r\n")
	if err != nil {
		return err
	}

	return nil
}

func configureBackends(rawBackends []*ingress.Backend) error {
	backends := make([]*ingress.Backend, len(rawBackends))
	for i, backend := range rawBackends {
		var service *apiv1.Service
		if backend.Service != nil {
			service = &apiv1.Service{Spec: backend.Service.Spec}
		}
		luaBackend := &ingress.Backend{
			Name:                 backend.Name,
			Port:                 backend.Port,
			SSLPassthrough:       backend.SSLPassthrough,
			SessionAffinity:      backend.SessionAffinity,
			UpstreamHashBy:       backend.UpstreamHashBy,
			LoadBalancing:        backend.LoadBalancing,
			Service:              service,
			NoServer:             backend.NoServer,
			TrafficShapingPolicy: backend.TrafficShapingPolicy,
			AlternativeBackends:  backend.AlternativeBackends,
		}

		var endpoints []ingress.Endpoint
		for _, endpoint := range backend.Endpoints {
			endpoints = append(endpoints, ingress.Endpoint{
				Address: endpoint.Address,
				Port:    endpoint.Port,
			})
		}

		luaBackend.Endpoints = endpoints
		backends[i] = luaBackend
	}

	statusCode, _, err := nginx.NewPostStatusRequest("/configuration/backends", "application/json", backends)
	if err != nil {
		return err
	}

	if statusCode != http.StatusCreated {
		return fmt.Errorf("unexpected error code: %d", statusCode)
	}

	return nil
}

type sslConfiguration struct {
	Certificates map[string]string   `json:"certificates"`
	Servers      map[string][]string `json:"servers"`
}

// configureCertificates JSON encodes certificates and POSTs it to an internal HTTP endpoint
// that is handled by Lua
func configureCertificates(rawServers []*ingress.Server) error {
	configuration := &sslConfiguration{
		Certificates: map[string]string{},
		Servers:      map[string][]string{},
	}

	configure := func(hostname string, sslCert *ingress.SSLCert) {
		uid := emptyUID

		if sslCert != nil {
			uid = sslCert.UID

			if _, ok := configuration.Certificates[uid]; !ok {
				configuration.Certificates[uid] = sslCert.PemCertKey
			}
		}

		configuration.Servers[hostname] = append(configuration.Servers[hostname], uid)
	}

	for _, rawServer := range rawServers {
		if len(rawServer.SSLCerts) != 0 {
			for _, sslCert := range rawServer.SSLCerts {
				configure(rawServer.Hostname, sslCert)
				for _, alias := range rawServer.Aliases {
					if sslCert != nil && ssl.IsValidHostname(alias, sslCert.CN) {
						configuration.Servers[alias] = append(configuration.Servers[alias], sslCert.UID)
					} else {
						configuration.Servers[alias] = append(configuration.Servers[alias], emptyUID)
					}
				}
			}
		} else {
			configure(rawServer.Hostname, nil)
			for _, alias := range rawServer.Aliases {
				configuration.Servers[alias] = append(configuration.Servers[alias], emptyUID)
			}
		}
	}

	redirects := buildRedirects(rawServers)
	for _, redirect := range redirects {
		if len(redirect.SSLCerts) != 0 {
			for _, sslCert := range redirect.SSLCerts {
				configure(redirect.From, sslCert)
			}
		} else {
			configure(redirect.From, nil)
		}
	}

	statusCode, _, err := nginx.NewPostStatusRequest("/configuration/servers", "application/json", configuration)
	if err != nil {
		return err
	}

	if statusCode != http.StatusCreated {
		return fmt.Errorf("unexpected error code: %d", statusCode)
	}

	return nil
}

const zipkinTmpl = `{
  "service_name": "{{ .ZipkinServiceName }}",
  "collector_host": "{{ .ZipkinCollectorHost }}",
  "collector_port": {{ .ZipkinCollectorPort }},
  "sample_rate": {{ .ZipkinSampleRate }}
}`

const jaegerTmpl = `{
  "service_name": "{{ .JaegerServiceName }}",
  "sampler": {
	"type": "{{ .JaegerSamplerType }}",
	"param": {{ .JaegerSamplerParam }},
	"samplingServerURL": "{{ .JaegerSamplerHost }}:{{ .JaegerSamplerPort }}/sampling"
  },
  "reporter": {
	"localAgentHostPort": "{{ .JaegerCollectorHost }}:{{ .JaegerCollectorPort }}"
  },
  "headers": {
	"TraceContextHeaderName": "{{ .JaegerTraceContextHeaderName }}",
	"jaegerDebugHeader": "{{ .JaegerDebugHeader }}",
	"jaegerBaggageHeader": "{{ .JaegerBaggageHeader }}",
	"traceBaggageHeaderPrefix": "{{ .JaegerTraceBaggageHeaderPrefix }}"
  },
}`

const datadogTmpl = `{
  "service": "{{ .DatadogServiceName }}",
  "agent_host": "{{ .DatadogCollectorHost }}",
  "agent_port": {{ .DatadogCollectorPort }},
  "operation_name_override": "{{ .DatadogOperationNameOverride }}",
  "sample_rate": {{ .DatadogSampleRate }},
  "dd.priority.sampling": {{ .DatadogPrioritySampling }}
}`

func createOpentracingCfg(cfg ngx_config.Configuration) error {
	var tmpl *template.Template
	var err error

	if cfg.ZipkinCollectorHost != "" {
		tmpl, err = template.New("zipkin").Parse(zipkinTmpl)
		if err != nil {
			return err
		}
	} else if cfg.JaegerCollectorHost != "" {
		tmpl, err = template.New("jaeger").Parse(jaegerTmpl)
		if err != nil {
			return err
		}
	} else if cfg.DatadogCollectorHost != "" {
		tmpl, err = template.New("datadog").Parse(datadogTmpl)
		if err != nil {
			return err
		}
	} else {
		tmpl, _ = template.New("empty").Parse("{}")
	}

	tmplBuf := bytes.NewBuffer(make([]byte, 0))
	err = tmpl.Execute(tmplBuf, cfg)
	if err != nil {
		return err
	}

	// Expand possible environment variables before writing the configuration to file.
	expanded := os.ExpandEnv(tmplBuf.String())

	return ioutil.WriteFile("/etc/nginx/opentracing.json", []byte(expanded), file.ReadWriteByUser)
}

func cleanTempNginxCfg() error {
	var files []string

	err := filepath.Walk(os.TempDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && os.TempDir() != path {
			return filepath.SkipDir
		}

		dur, _ := time.ParseDuration("-5m")
		fiveMinutesAgo := time.Now().Add(dur)
		if strings.HasPrefix(info.Name(), tempNginxPattern) && info.ModTime().Before(fiveMinutesAgo) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			return err
		}
	}

	return nil
}

type redirect struct {
	From     string
	To       string
	SSLCerts []*ingress.SSLCert
}

func buildRedirects(servers []*ingress.Server) []*redirect {
	names := sets.String{}
	redirectServers := make([]*redirect, 0)

	for _, srv := range servers {
		if !srv.RedirectFromToWWW {
			continue
		}

		to := srv.Hostname

		var from string
		if strings.HasPrefix(to, "www.") {
			from = strings.TrimPrefix(to, "www.")
		} else {
			from = fmt.Sprintf("www.%v", to)
		}

		if names.Has(to) {
			continue
		}

		klog.V(3).Infof("Creating redirect from %q to %q", from, to)
		found := false
		for _, esrv := range servers {
			if esrv.Hostname == from {
				found = true
				break
			}
		}

		if found {
			klog.Warningf("Already exists an Ingress with %q hostname. Skipping creation of redirection from %q to %q.", from, from, to)
			continue
		}

		r := &redirect{
			From: from,
			To:   to,
		}

		if len(srv.SSLCerts) != 0 {
			for _, sslCert := range srv.SSLCerts {
				if ssl.IsValidHostname(from, sslCert.CN) {
					r.SSLCerts = srv.SSLCerts
					klog.Infof("the server %v has the SSL certificate that contains a CN for %v.", from, to)
					break
				}
			}
			if len(r.SSLCerts) == 0 {
				klog.Warningf("the server %v has SSL configured but the SSL certificate does not contains a CN for %v. Redirects will not work for HTTPS to HTTPS", from, to)
			}
		}

		redirectServers = append(redirectServers, r)
		names.Insert(to)
	}

	return redirectServers
}

type DefaultServer struct {
	Hostname     string
	Port         int32
	DefaultCerts []*ingress.SSLCert
}

func buildDefaultServers(servers []*ingress.Server, defaultCertPorts string) []*DefaultServer {
	defaultServers := make([]*DefaultServer, 0)

	var defaultCertPortsMap map[string]int32
	err := json.Unmarshal([]byte(defaultCertPorts), &defaultCertPortsMap)
	if err != nil {
		klog.Warningf("buildDefaultServers: decode ports map failed [%v], error [%v]", defaultCertPorts, err)
	}

	for _, srv := range servers {
		if !srv.NeedDefaultCert {
			continue
		}

		var port int32 = -1
		var sslCert *ingress.SSLCert
		for _, sslCert = range srv.SSLCerts {
			p, exist := defaultCertPortsMap[sslCert.Name]
			if exist {
				port = p
				break
			}
		}

		if port == -1 {
			klog.Warningf("buildDefaultServers: cert [%v] not exist on configmap default-cert-ports", sslCert.Name)
			srv.NeedDefaultCert = false
			continue
		}

		srv.DefaultCertPort = int(port)
		klog.Warningf("buildDefaultServers: srv %v, port %v", sslCert.Name, srv.DefaultCertPort)

		found := false
		var cur *DefaultServer
		for _, d := range defaultServers {
			if d.Port == port {
				cur = d
				found = true
				break
			}
		}

		if found {
			foundCert := false
			for _, f := range cur.DefaultCerts {
				if f.Name == sslCert.Name {
					foundCert = true
					break
				}
			}

			//just add cert
			if !foundCert {
				pemFileName, err := ssl.StoreSSLCertOnDisk(sslCert.Name, sslCert)
				if err != nil {
					klog.Warningf("buildDefaultServers: cert %v store disk error: %v", sslCert.Name, err)
					continue
				}
				sslCert.PemFileName = pemFileName
				cur.DefaultCerts = append(cur.DefaultCerts, sslCert)
			}

			continue
		}

		//new default server
		pemFileName, err := ssl.StoreSSLCertOnDisk(sslCert.Name, sslCert)
		if err != nil {
			klog.Warningf("buildDefaultServers: cert %v store disk error: %v", sslCert.Name, err)
			continue
		}
		sslCert.PemFileName = pemFileName

		defaultCerts := make([]*ingress.SSLCert, 0)
		defaultCerts = append(defaultCerts, sslCert)

		d := &DefaultServer{
			Port:         defaultCertPortsMap[sslCert.Name],
			Hostname:     "_",
			DefaultCerts: defaultCerts,
		}

		defaultServers = append(defaultServers, d)
	}

	return defaultServers
}
