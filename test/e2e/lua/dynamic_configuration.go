/*
Copyright 2018 The Kubernetes Authors.

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

package lua

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/ingress-nginx/internal/nginx"
	"k8s.io/ingress-nginx/test/e2e/framework"
)

const (
	logDynamicConfigSuccess = "Dynamic reconfiguration succeeded"
	logDynamicConfigFailure = "Dynamic reconfiguration failed"
	logRequireBackendReload = "Configuration changes detected, backend reload required"
	logBackendReloadSuccess = "Backend successfully reloaded"
	logInitialConfigSync    = "Initial synchronization of the NGINX configuration"

	waitForLuaSync = 5 * time.Second
)

var _ = framework.IngressNginxDescribe("[Lua] dynamic configuration", func() {
	f := framework.NewDefaultFramework("dynamic-configuration")

	ginkgo.BeforeEach(func() {
		f.NewEchoDeploymentWithReplicas(1)
		ensureIngress(f, "foo.com", framework.EchoService)
	})

	ginkgo.It("configures balancer Lua middleware correctly", func() {
		f.WaitForNginxConfiguration(func(cfg string) bool {
			return strings.Contains(cfg, "balancer.init_worker()") && strings.Contains(cfg, "balancer.balance()")
		})

		host := "foo.com"
		f.WaitForNginxServer(host, func(server string) bool {
			return strings.Contains(server, "balancer.rewrite()") && strings.Contains(server, "balancer.log()")
		})
	})

	ginkgo.Context("when only backends change", func() {
		ginkgo.It("handles endpoints only changes", func() {
			var nginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				nginxConfig = cfg
				return true
			})

			replicas := 2
			err := framework.UpdateDeployment(f.KubeClientSet, f.Namespace, framework.EchoService, replicas, nil)
			assert.Nil(ginkgo.GinkgoT(), err)

			f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "foo.com").
				Expect().
				Status(http.StatusOK)

			var newNginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				newNginxConfig = cfg
				return true
			})
			assert.Equal(ginkgo.GinkgoT(), nginxConfig, newNginxConfig)
		})

		ginkgo.It("handles endpoints only changes (down scaling of replicas)", func() {
			var nginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				nginxConfig = cfg
				return true
			})

			replicas := 2
			err := framework.UpdateDeployment(f.KubeClientSet, f.Namespace, framework.EchoService, replicas, nil)
			assert.Nil(ginkgo.GinkgoT(), err)

			time.Sleep(waitForLuaSync)

			f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "foo.com").
				Expect().
				Status(http.StatusOK)

			var newNginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				newNginxConfig = cfg
				return true
			})
			assert.Equal(ginkgo.GinkgoT(), nginxConfig, newNginxConfig)

			err = framework.UpdateDeployment(f.KubeClientSet, f.Namespace, framework.EchoService, 0, nil)
			assert.Nil(ginkgo.GinkgoT(), err)

			time.Sleep(waitForLuaSync)

			f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "foo.com").
				Expect().
				Status(503)
		})

		ginkgo.It("handles endpoints only changes consistently (down scaling of replicas vs. empty service)", func() {
			deploymentName := "scalingecho"
			f.NewEchoDeploymentWithNameAndReplicas(deploymentName, 0)
			createIngress(f, "scaling.foo.com", deploymentName)

			resp := f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "scaling.foo.com").
				Expect().Raw()

			originalResponseCode := resp.StatusCode

			replicas := 2
			err := framework.UpdateDeployment(f.KubeClientSet, f.Namespace, deploymentName, replicas, nil)
			assert.Nil(ginkgo.GinkgoT(), err)

			time.Sleep(waitForLuaSync)

			resp = f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "scaling.foo.com").
				Expect().Raw()

			expectedSuccessResponseCode := resp.StatusCode

			replicas = 0
			err = framework.UpdateDeployment(f.KubeClientSet, f.Namespace, deploymentName, replicas, nil)
			assert.Nil(ginkgo.GinkgoT(), err)

			time.Sleep(waitForLuaSync)

			resp = f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "scaling.foo.com").
				Expect().Raw()

			expectedFailureResponseCode := resp.StatusCode

			assert.Equal(ginkgo.GinkgoT(), originalResponseCode, 503, "Expected empty service to return 503 response.")
			assert.Equal(ginkgo.GinkgoT(), expectedFailureResponseCode, 503, "Expected downscaled replicaset to return 503 response.")
			assert.Equal(ginkgo.GinkgoT(), expectedSuccessResponseCode, 200, "Expected intermediate scaled replicaset to return a 200 response.")
		})

		ginkgo.It("handles an annotation change", func() {
			var nginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				nginxConfig = cfg
				return true
			})

			ingress, err := f.KubeClientSet.NetworkingV1beta1().Ingresses(f.Namespace).Get("foo.com", metav1.GetOptions{})
			assert.Nil(ginkgo.GinkgoT(), err)

			ingress.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/load-balance"] = "round_robin"
			_, err = f.KubeClientSet.NetworkingV1beta1().Ingresses(f.Namespace).Update(ingress)
			assert.Nil(ginkgo.GinkgoT(), err)

			f.HTTPTestClient().
				GET("/").
				WithHeader("Host", "foo.com").
				Expect().
				Status(http.StatusOK)

			var newNginxConfig string
			f.WaitForNginxConfiguration(func(cfg string) bool {
				newNginxConfig = cfg
				return true
			})

			assert.Equal(ginkgo.GinkgoT(), nginxConfig, newNginxConfig)
		})
	})

	ginkgo.It("sets controllerPodsCount in Lua general configuration", func() {
		// https://github.com/curl/curl/issues/936
		curlCmd := fmt.Sprintf("curl --fail --silent http://localhost:%v/configuration/general", nginx.StatusPort)

		output, err := f.ExecIngressPod(curlCmd)
		assert.Nil(ginkgo.GinkgoT(), err)
		assert.Equal(ginkgo.GinkgoT(), output, `{"controllerPodsCount":1}`)

		err = framework.UpdateDeployment(f.KubeClientSet, f.Namespace, "nginx-ingress-controller", 3, nil)
		assert.Nil(ginkgo.GinkgoT(), err)

		output, err = f.ExecIngressPod(curlCmd)
		assert.Nil(ginkgo.GinkgoT(), err)
		assert.Equal(ginkgo.GinkgoT(), output, `{"controllerPodsCount":3}`)
	})
})

func ensureIngress(f *framework.Framework, host string, deploymentName string) *networking.Ingress {
	ing := createIngress(f, host, deploymentName)

	f.HTTPTestClient().
		GET("/").
		WithHeader("Host", host).
		Expect().
		Status(http.StatusOK)

	return ing
}

func createIngress(f *framework.Framework, host string, deploymentName string) *networking.Ingress {
	ing := f.EnsureIngress(framework.NewSingleIngress(host, "/", host, f.Namespace, deploymentName, 80,
		map[string]string{
			"nginx.ingress.kubernetes.io/load-balance": "ewma",
		},
	))

	f.WaitForNginxServer(host,
		func(server string) bool {
			return strings.Contains(server, fmt.Sprintf("server_name %s ;", host)) &&
				strings.Contains(server, "proxy_pass http://upstream_balancer;")
		})

	return ing
}

func ensureHTTPSRequest(f *framework.Framework, url string, host string, expectedDNSName string) {
	resp := f.HTTPTestClientWithTLSConfig(&tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}).
		GET("/").
		WithURL(url).
		WithHeader("Host", host).
		Expect().
		Raw()

	assert.Equal(ginkgo.GinkgoT(), resp.StatusCode, http.StatusOK)
	assert.Equal(ginkgo.GinkgoT(), len(resp.TLS.PeerCertificates), 1)
	assert.Equal(ginkgo.GinkgoT(), resp.TLS.PeerCertificates[0].DNSNames[0], expectedDNSName)
}
