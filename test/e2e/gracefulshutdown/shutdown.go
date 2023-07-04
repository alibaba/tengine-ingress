/*
Copyright 2019 The Kubernetes Authors.

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

package gracefulshutdown

import (
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/ingress-nginx/test/e2e/framework"
)

var _ = framework.IngressNginxDescribe("[Shutdown] ingress controller", func() {
	f := framework.NewDefaultFramework("shutdown-ingress-controller")

	host := "shutdown"

	ginkgo.BeforeEach(func() {
		f.UpdateNginxConfigMapData("worker-shutdown-timeout", "600s")

		f.NewSlowEchoDeployment()
	})

	ginkgo.It("should shutdown in less than 60 secons without pending connections", func() {
		defer ginkgo.GinkgoRecover()

		f.EnsureIngress(framework.NewSingleIngress(host, "/", host, f.Namespace, framework.SlowEchoService, 80, nil))

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, "server_name shutdown")
			})

		f.HTTPTestClient().
			GET("/sleep/1").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK)

		startTime := time.Now()

		f.ScaleDeploymentToZero("nginx-ingress-controller")

		assert.LessOrEqual(ginkgo.GinkgoT(), int(time.Since(startTime).Seconds()), 60, "waiting shutdown")
	})

	type asyncResult struct {
		status int
	}

	ginkgo.It("should shutdown after waiting 60 seconds for pending connections to be closed", func() {
		defer ginkgo.GinkgoRecover()

		err := framework.UpdateDeployment(f.KubeClientSet, f.Namespace, "nginx-ingress-controller", 1,
			func(deployment *appsv1.Deployment) error {
				grace := int64(3600)
				deployment.Spec.Template.Spec.TerminationGracePeriodSeconds = &grace
				_, err := f.KubeClientSet.AppsV1().Deployments(f.Namespace).Update(deployment)
				return err
			})

		assert.Nil(ginkgo.GinkgoT(), err)

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/proxy-send-timeout": "600",
			"nginx.ingress.kubernetes.io/proxy-read-timeout": "600",
		}
		f.EnsureIngress(framework.NewSingleIngress(host, "/", host, f.Namespace, framework.SlowEchoService, 80, annotations))

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, "server_name shutdown")
			})

		result := make(chan *asyncResult)

		startTime := time.Now()

		go func(host string, c chan *asyncResult) {
			defer ginkgo.GinkgoRecover()

			resp := f.HTTPTestClient().
				GET("/sleep/70").
				WithHeader("Host", host).
				Expect().
				Raw()

			code := 0
			if resp != nil {
				code = resp.StatusCode
			}

			c <- &asyncResult{code}
		}(host, result)

		time.Sleep(5 * time.Second)

		f.ScaleDeploymentToZero("nginx-ingress-controller")

		ticker := time.NewTicker(time.Second * 10)

		for {
			select {
			case res := <-result:
				assert.Equal(ginkgo.GinkgoT(), res.status, http.StatusOK, "expecting a valid response from HTTP request")
				assert.GreaterOrEqual(ginkgo.GinkgoT(), int(time.Since(startTime).Seconds()), 60, "waiting shutdown")
				ticker.Stop()
				return
			case <-ticker.C:
				framework.Logf("waiting for request completion after shutdown")
			}
		}
	})

	ginkgo.It("should shutdown after waiting 150 seconds for pending connections to be closed", func() {
		err := framework.UpdateDeployment(f.KubeClientSet, f.Namespace, "nginx-ingress-controller", 1,
			func(deployment *appsv1.Deployment) error {
				grace := int64(3600)
				deployment.Spec.Template.Spec.TerminationGracePeriodSeconds = &grace
				_, err := f.KubeClientSet.AppsV1().Deployments(f.Namespace).Update(deployment)
				return err
			})
		assert.Nil(ginkgo.GinkgoT(), err)

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/proxy-send-timeout": "600",
			"nginx.ingress.kubernetes.io/proxy-read-timeout": "600",
		}
		f.EnsureIngress(framework.NewSingleIngress(host, "/", host, f.Namespace, framework.SlowEchoService, 80, annotations))

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, "server_name shutdown")
			})

		result := make(chan *asyncResult)

		startTime := time.Now()

		go func(host string, c chan *asyncResult) {
			defer ginkgo.GinkgoRecover()

			resp := f.HTTPTestClient().
				GET("/sleep/150").
				WithHeader("Host", host).
				Expect().
				Raw()

			code := 0
			if resp != nil {
				code = resp.StatusCode
			}

			c <- &asyncResult{code}
		}(host, result)

		time.Sleep(5 * time.Second)

		f.ScaleDeploymentToZero("nginx-ingress-controller")

		ticker := time.NewTicker(time.Second * 10)

		for {
			select {
			case res := <-result:
				assert.Equal(ginkgo.GinkgoT(), res.status, http.StatusOK, "expecting a valid response from HTTP request")
				assert.GreaterOrEqual(ginkgo.GinkgoT(), int(time.Since(startTime).Seconds()), 150, "waiting shutdown")
				ticker.Stop()
				return
			case <-ticker.C:
				framework.Logf("waiting for request completion after shutdown")
			}
		}
	})
})
