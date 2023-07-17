# Tengine-Ingress

[![GitHub license](https://img.shields.io/github/license/alibaba/tengine-ingress.svg)](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
[![GitHub stars](https://img.shields.io/badge/contributions-welcome-orange.svg)](https://github.com/alibaba/tengine-ingress/blob/main/CONTRIBUTING.md)

## Overview

Tengine-Ingress is an Ingress controller for Kubernetes using [Tengine](https://github.com/alibaba/tengine) as a reverse proxy and load balancer.
**Note**: Tengine-Ingress supports the standard Ingress specification based on [kubernetes/ingress-nginx](https://github.com/kubernetes/ingress-nginx) repo.


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

Use the docker image directly:

```
docker pull tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.0.0
```

This docker image is based the linux distribution [anolisos](https://hub.docker.com/r/openanolis/anolisos). Support arm64 and amd64 architectures.

If recompilation is required, it can be done by the following docker build command. The ingress image is based on the tengine image. Currently only supports compilation based on the base image [anolisos](https://hub.docker.com/r/openanolis/anolisos).

```
# tengine image build command
docker build --no-cache --build-arg BASE_IMAGE="docker.io/openanolis/anolisos:latest" --build-arg LINUX_RELEASE="anolisos" -t tengine:3.0.0 images/tengine/rootfs/

# ingress controller image build command
docker build --no-cache --build-arg BASE_IMAGE="tengine:3.0.0" --build-arg VERSION="1.0.0" -f build/Dockerfile -t tengine-ingress:1.0.0 .
```

## Documentation

The homepage of Tengine is at [http://tengine.taobao.org/](http://tengine.taobao.org/)


## Contact

[https://github.com/alibaba/tengine-ingress/issues](https://github.com/alibaba/tengine-ingress/issues)

Dingtalk user group: 23394285


## License

[Apache License 2.0](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
