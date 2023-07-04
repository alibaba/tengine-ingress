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

package annotations

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/stretchr/testify/assert"
	"k8s.io/ingress-nginx/test/e2e/framework"
)

var _ = framework.DescribeAnnotation("auth-tls-*", func() {
	f := framework.NewDefaultFramework("authtls")

	ginkgo.BeforeEach(func() {
		f.NewEchoDeploymentWithReplicas(2)
	})

	ginkgo.It("should set valid auth-tls-secret", func() {
		host := "authtls.foo.com"
		nameSpace := f.Namespace

		clientConfig, err := framework.CreateIngressMASecret(
			f.KubeClientSet,
			host,
			host,
			nameSpace)
		assert.Nil(ginkgo.GinkgoT(), err)

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret": nameSpace + "/" + host,
		}

		f.EnsureIngress(framework.NewSingleIngressWithTLS(host, "/", host, []string{host}, nameSpace, framework.EchoService, 80, annotations))

		assertSslClientCertificateConfig(f, host, "on", "1")

		// Send Request without Client Certs
		f.HTTPTestClientWithTLSConfig(&tls.Config{ServerName: host, InsecureSkipVerify: true}).
			GET("/").
			WithURL(f.GetURL(framework.HTTPS)).
			WithHeader("Host", host).
			Expect().
			Status(http.StatusBadRequest)

		// Send Request Passing the Client Certs
		f.HTTPTestClientWithTLSConfig(clientConfig).
			GET("/").
			WithURL(f.GetURL(framework.HTTPS)).
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK)
	})

	ginkgo.It("should set valid auth-tls-secret, sslVerify to off, and sslVerifyDepth to 2", func() {
		host := "authtls.foo.com"
		nameSpace := f.Namespace

		_, err := framework.CreateIngressMASecret(
			f.KubeClientSet,
			host,
			host,
			nameSpace)
		assert.Nil(ginkgo.GinkgoT(), err)

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret":        nameSpace + "/" + host,
			"nginx.ingress.kubernetes.io/auth-tls-verify-client": "off",
			"nginx.ingress.kubernetes.io/auth-tls-verify-depth":  "2",
		}

		f.EnsureIngress(framework.NewSingleIngressWithTLS(host, "/", host, []string{host}, nameSpace, framework.EchoService, 80, annotations))

		assertSslClientCertificateConfig(f, host, "off", "2")

		// Send Request without Client Certs
		f.HTTPTestClient().
			GET("/").
			WithURL(f.GetURL(framework.HTTPS)).
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK)
	})

	ginkgo.It("should set valid auth-tls-secret, pass certificate to upstream, and error page", func() {
		host := "authtls.foo.com"
		nameSpace := f.Namespace

		errorPath := "/error"

		clientConfig, err := framework.CreateIngressMASecret(
			f.KubeClientSet,
			host,
			host,
			nameSpace)
		assert.Nil(ginkgo.GinkgoT(), err)

		annotations := map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret":                       nameSpace + "/" + host,
			"nginx.ingress.kubernetes.io/auth-tls-error-page":                   f.GetURL(framework.HTTP) + errorPath,
			"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
		}

		f.EnsureIngress(framework.NewSingleIngressWithTLS(host, "/", host, []string{host}, nameSpace, framework.EchoService, 80, annotations))

		assertSslClientCertificateConfig(f, host, "on", "1")

		sslErrorPage := fmt.Sprintf("error_page 495 496 = %s;", f.GetURL(framework.HTTP)+errorPath)
		sslUpstreamClientCert := "proxy_set_header ssl-client-cert $ssl_client_escaped_cert;"

		f.WaitForNginxServer(host,
			func(server string) bool {
				return strings.Contains(server, sslErrorPage) &&
					strings.Contains(server, sslUpstreamClientCert)
			})

		// Send Request without Client Certs
		f.HTTPTestClient().
			GET("/").
			WithURL(f.GetURL(framework.HTTPS)).
			WithHeader("Host", host).
			Expect().
			Status(http.StatusFound).
			Header("Location").Equal(f.GetURL(framework.HTTP) + errorPath)

		// Send Request Passing the Client Certs
		f.HTTPTestClientWithTLSConfig(clientConfig).
			GET("/").
			WithURL(f.GetURL(framework.HTTPS)).
			WithHeader("Host", host).
			Expect().
			Status(http.StatusOK)
	})
})

func assertSslClientCertificateConfig(f *framework.Framework, host string, verifyClient string, verifyDepth string) {
	sslClientCertDirective := fmt.Sprintf("ssl_client_certificate /etc/ingress-controller/ssl/%s-%s.pem;", f.Namespace, host)
	sslVerify := fmt.Sprintf("ssl_verify_client %s;", verifyClient)
	sslVerifyDepth := fmt.Sprintf("ssl_verify_depth %s;", verifyDepth)

	f.WaitForNginxServer(host,
		func(server string) bool {
			return strings.Contains(server, sslClientCertDirective) &&
				strings.Contains(server, sslVerify) &&
				strings.Contains(server, sslVerifyDepth)
		})
}
