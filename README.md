# Tengine-Ingress
[![GitHub license](https://img.shields.io/github/license/alibaba/tengine-ingress.svg)](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
[![GitHub stars](https://img.shields.io/badge/contributions-welcome-orange.svg)](https://github.com/alibaba/tengine-ingress/blob/main/CONTRIBUTING.md)

## Overview
Tengine-Ingress is an Ingress controller for Kubernetes using [Tengine](https://github.com/alibaba/tengine) as a reverse proxy and load balancer.
Tengine-Ingress supports the standard Ingress specification based on [kubernetes/ingress-nginx](https://github.com/kubernetes/ingress-nginx) repo.

## Features
* Dynamically reconfigure the servers, locations and upstreams for Ingress, Secret, Service and Endpoint changes, without reloading or restarting worker processes.
* Dynamically reconfigure canary routing based on standard and custom HTTP headers, header value, and weights.
* Dynamically reconfigure timeout setting, SSL Redirects, CORS and enabling/disabling robots for the ingress/path.
* Dynamically reconfigure certificates and keys.
* Support for hybrid ECC and RSA certificates for the same ingress/path.
* HTTP/3 support (QUIC v1 and draft-29).
* Supports watching Ingress and Secrets in a dedicated storage k8s cluster via kubeconfig.
* Watch changes in Ingress and Secrets and do rolling upgrades for associated StatefulSet of Tengine-Ingress, without tengine reload.
* New CRD IngressCheckSum and SecretCheckSum to verify the integrity of Ingress and Secret in the cluster.

## Installation
### Docker images
Supported operating systems:
* [Anolis](https://hub.docker.com/r/openanolis/anolisos)

Supported architectures:
* AMD64, ARM64
```
docker pull tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.0.0
```

### Building from source
The tengine-ingress image is based on the tengine image.

Supported Linux distributions:
* [Anolis](https://hub.docker.com/r/openanolis/anolisos), [Alpine](https://hub.docker.com/_/alpine)
```
# First: build tengine image
docker build --no-cache --build-arg BASE_IMAGE="docker.io/openanolis/anolisos:latest" --build-arg LINUX_RELEASE="anolisos" -t tengine:3.0.0 images/tengine/rootfs/

# Second: build tengine-ingress image
docker build --no-cache --build-arg BASE_IMAGE="tengine:3.0.0" --build-arg VERSION="1.0.0" -f build/Dockerfile -t tengine-ingress:1.0.0 .
```

## Changelog

See [the list of releases](https://github.com/alibaba/tengine-ingress/releases) to find out about feature changes.
For detailed changes for each release; please check the [Changelog.tengine.md](Changelog.tengine.md) file.

### Supported Versions table
|    | Tengine-Ingress Version | Tengine Version | K8s Supported Version | Anolis Linux Version | Alpine Linux version | Helm Chart Version |
|:--:|-------------------------|-----------------|-----------------------|----------------------|----------------------|--------------------|
| 🔄 | **v1.0.0**              | v3.0.0          | 1.27,1.26,1.25,1.24<br>1.23,1.22,1.21,1.20   | 8.6                  | 3.18.2               |                    |
| 🔄 |                         |                 |                       |                      |                      |                    |

## Documentation

The homepage of Tengine-Ingress is at [https://tengine.taobao.org](https://tengine.taobao.org/).

## Contact

[https://github.com/alibaba/tengine-ingress/issues](https://github.com/alibaba/tengine-ingress/issues)

Dingtalk user group: 23394285

## License

[Apache License 2.0](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
