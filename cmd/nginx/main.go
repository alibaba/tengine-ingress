/*
Copyright 2015 The Kubernetes Authors.
Copyright 2022-2023 The Alibaba Authors.

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

package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	discovery "k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"

	ingcheck "k8s.io/ingress-nginx/internal/checksum/ingress/client/clientset/versioned"
	secretcheck "k8s.io/ingress-nginx/internal/checksum/secret/client/clientset/versioned"
	"k8s.io/ingress-nginx/internal/file"
	"k8s.io/ingress-nginx/internal/ingress/controller"
	"k8s.io/ingress-nginx/internal/ingress/metric"
	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/internal/net/ssl"
	"k8s.io/ingress-nginx/internal/nginx"
	"k8s.io/ingress-nginx/version"
)

func main() {
	klog.InitFlags(nil)

	rand.Seed(time.Now().UnixNano())

	fmt.Println(version.String())

	showVersion, conf, err := parseFlags()
	if showVersion {
		os.Exit(0)
	}

	if err != nil {
		klog.Fatal(err)
	}

	err = file.CreateRequiredDirectories()
	if err != nil {
		klog.Fatal(err)
	}

	klog.Infof("Create apiserver client for common resource")
	kubeClient := createApiServerClient(conf.APIServerHost, conf.RootCAFile, "")

	// apiserver client for ingress and secret in a dedicated storage k8s cluster
	klog.Infof("Create apiserver client for ingress resource")
	kubeIngClient := createApiServerClient("", "", conf.KubeConfigFile)

	// apiserver client for ingress checksum in a dedicated storage k8s cluster
	klog.Infof("Create apiserver client for ingress check")
	kubeIngCheckClient := createIngCrdApiServerClient("", "", conf.KubeConfigFile)

	// apiserver client for secret checksum in a dedicated storage k8s cluster
	klog.Infof("Create apiserver client for secret check")
	kubeSecretCheckClient := createSecretCrdApiServerClient("", "", conf.KubeConfigFile)

	if len(conf.DefaultService) > 0 {
		defSvcNs, defSvcName, err := k8s.ParseNameNS(conf.DefaultService)
		if err != nil {
			klog.Fatal(err)
		}

		_, err = kubeClient.CoreV1().Services(defSvcNs).Get(context.TODO(), defSvcName, metav1.GetOptions{})
		if err != nil {
			if errors.IsUnauthorized(err) || errors.IsForbidden(err) {
				klog.Fatal("✖ The cluster seems to be running with a restrictive Authorization mode and the Ingress controller does not have the required permissions to operate normally.")
			}
			klog.Fatalf("No service with name %v found: %v", conf.DefaultService, err)
		}
		klog.Infof("Validated %v as the default backend.", conf.DefaultService)
	}

	if conf.Namespace != "" {
		_, err = kubeClient.CoreV1().Namespaces().Get(context.TODO(), conf.Namespace, metav1.GetOptions{})
		if err != nil {
			klog.Fatalf("No namespace with name %v found: %v", conf.Namespace, err)
		}
	}

	conf.FakeCertificate = ssl.GetFakeSSLCert()
	klog.Infof("SSL fake certificate created %v", conf.FakeCertificate.PemFileName)

	if !k8s.NetworkingIngressAvailable(kubeClient) {
		klog.Fatalf("tengine-ingress requires Kubernetes v1.19.0 or higher")
	}

	conf.Client = kubeClient
	conf.ClientIng = kubeIngClient
	conf.ClientIngCheck = kubeIngCheckClient
	conf.ClientSecretCheck = kubeSecretCheckClient

	reg := prometheus.NewRegistry()

	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{
		PidFn:        func() (int, error) { return os.Getpid(), nil },
		ReportErrors: true,
	}))

	mc := metric.NewDummyCollector()
	if conf.EnableMetrics {
		mc, err = metric.NewCollector(conf.MetricsPerHost, reg)
		if err != nil {
			klog.Fatalf("Error creating prometheus collector:  %v", err)
		}
	}
	mc.Start()

	if conf.EnableProfiling {
		go registerProfiler()
	}

	ngx := controller.NewNGINXController(conf, mc)

	mux := http.NewServeMux()
	registerHealthz(nginx.HealthPath, ngx, mux)
	registerMetrics(reg, mux)

	go startHTTPServer(conf.ListenPorts.Health, mux)
	go ngx.Start()

	handleSigterm(ngx, func(code int) {
		os.Exit(code)
	})
}

type exiter func(code int)

func handleSigterm(ngx *controller.NGINXController, exit exiter) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	<-signalChan
	klog.Info("Received SIGTERM, shutting down")

	exitCode := 0
	if err := ngx.Stop(); err != nil {
		klog.Infof("Error during shutdown: %v", err)
		exitCode = 1
	}

	klog.Info("Handled quit, awaiting Pod deletion")
	time.Sleep(10 * time.Second)

	klog.Infof("Exiting with %v", exitCode)
	exit(exitCode)
}

func createApiServerClient(apiserverHost, rootCAFile, kubeConfig string) *kubernetes.Clientset {
	kubeClient, err := createApiserverClient(apiserverHost, rootCAFile, kubeConfig)
	if err != nil {
		klog.Errorf("Failed to create apiserver client: %v", err)
		handleFatalInitError(err)
	}

	klog.Infof("Create apiserver client successfully")

	return kubeClient
}

func createIngCrdApiServerClient(apiserverHost, rootCAFile, kubeConfig string) *ingcheck.Clientset {
	kubeClient, err := createIngCrdApiserverClient(apiserverHost, rootCAFile, kubeConfig)
	if err != nil {
		klog.Errorf("Failed to create apiserver client for crd ingress checksum: %v", err)
		handleFatalInitError(err)
	}

	klog.Infof("Create apiserver client for crd ingress checksum successfully")

	return kubeClient
}

func createIngCrdApiserverClient(apiserverHost, rootCAFile, kubeConfig string) (*ingcheck.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiserverHost, kubeConfig)
	if err != nil {
		return nil, err
	}

	if apiserverHost != "" && rootCAFile != "" {
		tlsClientConfig := rest.TLSClientConfig{}

		if _, err := certutil.NewPool(rootCAFile); err != nil {
			klog.Errorf("Expected to load root CA config from %s, but got err: %v", rootCAFile, err)
		} else {
			tlsClientConfig.CAFile = rootCAFile
		}

		cfg.TLSClientConfig = tlsClientConfig
	}

	klog.Infof("Creating API client for %s", cfg.Host)

	client, err := ingcheck.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	var v *discovery.Info

	// The client may fail to connect to the API server in the first request.
	// https://github.com/kubernetes/ingress-nginx/issues/1968
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	klog.V(2).Info("Trying to discover Kubernetes version")
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		klog.V(2).Infof("Unexpected error discovering Kubernetes version (attempt %v): %v", retries, err)
		retries++
		return false, nil
	})

	// err is returned in case of timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	// this should not happen, warn the user
	if retries > 0 {
		klog.Warningf("Initial connection to the Kubernetes API server was retried %d times.", retries)
	}

	klog.Infof("Running in Kubernetes cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform)

	return client, nil
}

func createSecretCrdApiServerClient(apiserverHost, rootCAFile, kubeConfig string) *secretcheck.Clientset {
	kubeClient, err := createSecretCrdApiserverClient(apiserverHost, rootCAFile, kubeConfig)
	if err != nil {
		klog.Errorf("Failed to create apiserver client for crd secret checksum: %v", err)
		handleFatalInitError(err)
	}

	klog.Infof("Create apiserver client for crd secret checksum successfully")

	return kubeClient
}

func createSecretCrdApiserverClient(apiserverHost, rootCAFile, kubeConfig string) (*secretcheck.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiserverHost, kubeConfig)
	if err != nil {
		return nil, err
	}

	if apiserverHost != "" && rootCAFile != "" {
		tlsClientConfig := rest.TLSClientConfig{}

		if _, err := certutil.NewPool(rootCAFile); err != nil {
			klog.Errorf("Expected to load root CA config from %s, but got err: %v", rootCAFile, err)
		} else {
			tlsClientConfig.CAFile = rootCAFile
		}

		cfg.TLSClientConfig = tlsClientConfig
	}

	klog.Infof("Creating API client for %s", cfg.Host)

	client, err := secretcheck.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	var v *discovery.Info

	// The client may fail to connect to the API server in the first request.
	// https://github.com/kubernetes/ingress-nginx/issues/1968
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	klog.V(2).Info("Trying to discover Kubernetes version")
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		klog.V(2).Infof("Unexpected error discovering Kubernetes version (attempt %v): %v", retries, err)
		retries++
		return false, nil
	})

	// err is returned in case of timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	// this should not happen, warn the user
	if retries > 0 {
		klog.Warningf("Initial connection to the Kubernetes API server was retried %d times.", retries)
	}

	klog.Infof("Running in Kubernetes cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform)

	return client, nil
}

// createApiserverClient creates a new Kubernetes REST client. apiserverHost is
// the URL of the API server in the format protocol://address:port/pathPrefix,
// kubeConfig is the location of a kubeconfig file. If defined, the kubeconfig
// file is loaded first, the URL of the API server read from the file is then
// optionally overridden by the value of apiserverHost.
// If neither apiserverHost nor kubeConfig is passed in, we assume the
// controller runs inside Kubernetes and fallback to the in-cluster config. If
// the in-cluster config is missing or fails, we fallback to the default config.
func createApiserverClient(apiserverHost, rootCAFile, kubeConfig string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiserverHost, kubeConfig)
	if err != nil {
		return nil, err
	}

	if apiserverHost != "" && rootCAFile != "" {
		tlsClientConfig := rest.TLSClientConfig{}

		if _, err := certutil.NewPool(rootCAFile); err != nil {
			klog.Errorf("Expected to load root CA config from %s, but got err: %v", rootCAFile, err)
		} else {
			tlsClientConfig.CAFile = rootCAFile
		}

		cfg.TLSClientConfig = tlsClientConfig
	}

	klog.Infof("Creating API client for %s", cfg.Host)

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	var v *discovery.Info

	// The client may fail to connect to the API server in the first request.
	// https://github.com/kubernetes/ingress-nginx/issues/1968
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	klog.V(2).Info("Trying to discover Kubernetes version")
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		klog.V(2).Infof("Unexpected error discovering Kubernetes version (attempt %v): %v", retries, err)
		retries++
		return false, nil
	})

	// err is returned in case of timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	// this should not happen, warn the user
	if retries > 0 {
		klog.Warningf("Initial connection to the Kubernetes API server was retried %d times.", retries)
	}

	klog.Infof("Running in Kubernetes cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform)

	return client, nil
}

// Handler for fatal init errors. Prints a verbose error message and exits.
func handleFatalInitError(err error) {
	klog.Fatalf("Error while initiating a connection to the Kubernetes API server. "+
		"This could mean the cluster is misconfigured (e.g. it has invalid API server certificates "+
		"or Service Accounts configuration). Reason: %s\n"+
		"Refer to the troubleshooting guide for more information: "+
		"https://kubernetes.github.io/ingress-nginx/troubleshooting/",
		err)
}

func registerHealthz(healthPath string, ic *controller.NGINXController, mux *http.ServeMux) {
	// expose health check endpoint (/healthz)
	healthz.InstallPathHandler(mux,
		healthPath,
		healthz.PingHealthz,
		ic,
	)
}

func registerMetrics(reg *prometheus.Registry, mux *http.ServeMux) {
	mux.Handle(
		"/metrics",
		promhttp.InstrumentMetricHandler(
			reg,
			promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		),
	)

}

func registerProfiler() {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/heap", pprof.Index)
	mux.HandleFunc("/debug/pprof/mutex", pprof.Index)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Index)
	mux.HandleFunc("/debug/pprof/threadcreate", pprof.Index)
	mux.HandleFunc("/debug/pprof/block", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%v", nginx.ProfilerPort),
		Handler: mux,
	}
	klog.Fatal(server.ListenAndServe())
}

func startHTTPServer(port int, mux *http.ServeMux) {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%v", port),
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	klog.Fatal(server.ListenAndServe())
}
