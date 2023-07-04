/*
Copyright 2017 The Kubernetes Authors.

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

package annotations

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/ingress-nginx/test/e2e/framework"
)

var _ = framework.DescribeAnnotation("affinity session-cookie-name", func() {
	f := framework.NewDefaultFramework("affinity")

	ginkgo.BeforeEach(func() {
		f.NewEchoDeploymentWithReplicas(2)
	})

	ginkgo.It("should set sticky cookie SERVERID", func() {
		host := "sticky.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
		}

		ing := framework.NewSingleIngress(host, "/", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("SERVERID=")
	})

	ginkgo.It("should change cookie name on ingress definition change", func() {
		host := "change.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
		}

		ing := framework.NewSingleIngress(host, "/", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("SERVERID")

		ing.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "OTHERCOOKIENAME"

		_, err := f.KubeClientSet.NetworkingV1beta1().Ingresses(f.Namespace).Update(ing)
		assert.Nil(ginkgo.GinkgoT(), err, "updating ingress")
		time.Sleep(5 * time.Second)

		f.HTTPTestClient().
			GET("/").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("OTHERCOOKIENAME")
	})

	ginkgo.It("should set the path to /something on the generated cookie", func() {
		host := "path.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
		}

		ing := framework.NewSingleIngress(host, "/something", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/something").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("Path=/something")
	})

	ginkgo.It("does not set the path to / on the generated cookie if there's more than one rule referring to the same backend", func() {
		host := "morethanonerule.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
		}

		f.EnsureIngress(&networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        host,
				Namespace:   f.Namespace,
				Annotations: annotations,
			},
			Spec: networking.IngressSpec{
				Rules: []networking.IngressRule{
					{
						Host: host,
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path: "/something",
										Backend: networking.IngressBackend{
											ServiceName: framework.EchoService,
											ServicePort: intstr.FromInt(80),
										},
									},
									{
										Path: "/somewhereelese",
										Backend: networking.IngressBackend{
											ServiceName: framework.EchoService,
											ServicePort: intstr.FromInt(80),
										},
									},
								},
							},
						},
					},
				},
			},
		})

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/something").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("Path=/something")

		f.HTTPTestClient().
			GET("/somewhereelese").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("Path=/somewhereelese")
	})

	ginkgo.It("should set cookie with expires", func() {
		host := "cookieexpires.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":               "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name":    "ExpiresCookie",
			"nginx.ingress.kubernetes.io/session-cookie-expires": "172800",
			"nginx.ingress.kubernetes.io/session-cookie-max-age": "259200",
		}

		ing := framework.NewSingleIngress(host, "/", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		local, err := time.LoadLocation("GMT")
		assert.Nil(ginkgo.GinkgoT(), err, "loading GMT location")
		assert.NotNil(ginkgo.GinkgoT(), local, "expected a location but none returned")

		duration, _ := time.ParseDuration("48h")
		expected := time.Now().In(local).Add(duration).Format("Mon, 02-Jan-06 15:04")

		f.HTTPTestClient().
			GET("/").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains(fmt.Sprintf("Expires=%s", expected)).Contains("Max-Age=259200")
	})

	ginkgo.It("should work with use-regex annotation and session-cookie-path", func() {
		host := "useregex.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
			"nginx.ingress.kubernetes.io/use-regex":           "true",
			"nginx.ingress.kubernetes.io/session-cookie-path": "/foo/bar",
		}

		ing := framework.NewSingleIngress(host, "/foo/.*", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/foo/bar").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("Path=/foo/bar").Contains("SERVERID=")
	})

	ginkgo.It("should warn user when use-regex is true and session-cookie-path is not set", func() {
		host := "useregexwarn.foo.com"
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
			"nginx.ingress.kubernetes.io/use-regex":           "true",
		}

		ing := framework.NewSingleIngress(host, "/foo/.*", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, fmt.Sprintf("server_name %s ;", host))
			})

		f.HTTPTestClient().
			GET("/foo/bar").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK)

		logs, err := f.NginxLogs()
		assert.Nil(ginkgo.GinkgoT(), err, "obtaining nginx logs")
		assert.Contains(ginkgo.GinkgoT(), logs, `session-cookie-path should be set when use-regex is true`)
	})

	ginkgo.It("should not set affinity across all server locations when using separate ingresses", func() {
		host := "separate.foo.com"

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity": "cookie",
		}
		ing1 := framework.NewSingleIngress("ingress1", "/foo/bar", host, f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing1)

		ing2 := framework.NewSingleIngress("ingress2", "/foo", host, f.Namespace, framework.EchoService, 80, nil)
		f.EnsureIngress(ing2)

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, `location /foo/bar`) && strings.Contains(server, `location /foo`)
			})

		f.HTTPTestClient().
			GET("/foo").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Empty()

		f.HTTPTestClient().
			GET("/foo/bar").
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("Path=/foo/bar")
	})

	ginkgo.It("should set sticky cookie without host", func() {
		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/affinity":            "cookie",
			"nginx.ingress.kubernetes.io/session-cookie-name": "SERVERID",
		}

		ing := framework.NewSingleIngress("default-no-host", "/", "", f.Namespace, framework.EchoService, 80, annotations)
		f.EnsureIngress(ing)

		f.WaitForNginxServer("_",
			func(server string) bool {
				return strings.Contains(server, "server_name _")
			})

		f.HTTPTestClient().
			GET("/").
			Expect().
			Status(http.StatusOK).
			Header("Set-Cookie").Contains("SERVERID=")
	})
})
