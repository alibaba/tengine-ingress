<h1 align="center" style="border-bottom: none">
    <br>Tengine-Ingress
</h1>

<p align="center">Visit <a href="https://tengine.taobao.org" target="_blank">tengine.taobao.org</a> for the full documentation,
examples and guides.</p>

<div align="center">

[![GitHub license](https://img.shields.io/github/license/alibaba/tengine-ingress.svg)](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/alibaba/tengine-ingress.svg)](https://github.com/alibaba/tengine-ingress/stargazers)
[![GitHub stars](https://img.shields.io/badge/contributions-welcome-orange.svg)](https://github.com/alibaba/tengine-ingress/blob/main/CONTRIBUTING.md)

</div>

## Overview
Tengine-Ingress is an Ingress controller for Kubernetes using [Tengine](https://github.com/alibaba/tengine) as a reverse proxy and load balancer.
Tengine-Ingress supports the standard Ingress specification based on [kubernetes/ingress-nginx](https://github.com/kubernetes/ingress-nginx) repo.

## Features
* Dynamically configure the servers, locations and upstreams for Ingress, Secret, Service and Endpoint changes, without reloading or restarting worker processes.
* HTTP/3 support (QUIC v1 and draft-29).
* Dynamically configure different TLS protocols for different server names.
* Dynamically configure multiple default TLS certificates for client-hello without SNI.
* Support for hybrid ECC and RSA certificates for the same ingress/path.
* Dynamically configure certificates and keys.
* Dynamically configure canary routing based on multiple values of a specific header, cookie or query parameter.
* Dynamically configure canary routing based on multiple upstream according to weight.
* Dynamically configure timeout setting, SSL Redirects, CORS and enabling/disabling robots for the ingress/path.
* Dynamically configure canary routing to add/append custom headers or add query parameter to the HTTP request.
* Dynamically configure canary routing to add custom headers to the HTTP response.
* Supports watching Ingress and Secrets in a dedicated storage k8s cluster via kubeconfig.
* Watch changes in Ingress and Secrets and do rolling upgrades for associated StatefulSet of Tengine-Ingress, without tengine reload.
* New CRD IngressCheckSum and SecretCheckSum to verify the integrity of Ingress and Secret in the cluster.

## Installation
### Docker images
Supported linux distributions:
* [Anolis](https://hub.docker.com/r/openanolis/anolisos)
* [Alpine](https://hub.docker.com/_/alpine)

Supported tags:
* `1.1.0` : based on image [Anolis](https://hub.docker.com/r/openanolis/anolisos)
* `1.1.0-alpine` : based on image [Alpine](https://hub.docker.com/_/alpine)

Supported architectures:
* AMD64, ARM64

Pull image command:

from docker.io mirror:
```
docker pull tengineimages/tengine-ingress:1.1.0
```

from aliyun mirror:
```
docker pull tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.1.0
```

### Building from source
The tengine-ingress image is based on the tengine image.

Supported Linux distributions:
* [Anolis](https://hub.docker.com/r/openanolis/anolisos) : build arg `BASE_IMAGE="docker.io/openanolis/anolisos:latest"`, `LINUX_RELEASE="anolisos"`
* [Alpine](https://hub.docker.com/_/alpine) : build arg `BASE_IMAGE="alpine:latest"`, `LINUX_RELEASE="alpine"`

Build image command:
```
# First: build tengine image
docker build --no-cache --build-arg BASE_IMAGE="docker.io/openanolis/anolisos:latest" --build-arg LINUX_RELEASE="anolisos" -t tengine:3.1.0 images/tengine/rootfs/

# Second: build tengine-ingress image
docker build --no-cache --build-arg BASE_IMAGE="tengine:3.1.0" --build-arg VERSION="1.1.0" -f build/Dockerfile -t tengine-ingress:1.1.0 .
```

## Changelog

See [the list of releases](https://github.com/alibaba/tengine-ingress/releases) to find out about feature changes.
For detailed changes for each release; please check the [Changelog.tengine.md](Changelog.tengine.md) file.

### Supported Versions table
|    | Tengine-Ingress Version | Tengine Version | K8s Supported Version | Anolis Linux Version | Alpine Linux Version | Helm Chart Version |
|:--:|-------------------------|-----------------|-----------------------|----------------------|----------------------|--------------------|
| 🔄 | **v1.1.0**              | v3.1.0          | 1.28,1.27,1.26,1.25<br>1.24,1.23,1.22,1.21<br>1.20 | 8.6                  | 3.18.4               |                    |
| 🔄 | **v1.0.0**              | v3.0.0          | 1.27,1.26,1.25,1.24<br>1.23,1.22,1.21,1.20   | 8.6                  | 3.18.2               |                    |

## Documentation

The homepage of Tengine-Ingress is at [https://tengine.taobao.org](https://tengine.taobao.org/).

## Contact

[https://github.com/alibaba/tengine-ingress/issues](https://github.com/alibaba/tengine-ingress/issues)

Dingtalk user group: 23394285

## License

[Apache License 2.0](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)

<h1 align="center" style="border-bottom: none">
    <a href="https://tengine.taobao.org" target="_blank"><img alt="Tengine-Ingress" src="/docs/images/tengine-logo.png"></a>
</h1>
