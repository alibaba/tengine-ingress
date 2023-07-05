# Tengine-Ingress

[![GitHub license](https://img.shields.io/github/license/alibaba/tengine-ingress.svg)](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)
[![GitHub stars](https://img.shields.io/badge/contributions-welcome-orange.svg)](https://github.com/alibaba/tengine-ingress/blob/main/CONTRIBUTING.md)


## Overview

Tengine-Ingress is an Ingress controller for Kubernetes using [Tengine](https://github.com/alibaba/tengine) as a reverse proxy and load balancer.
**Note**: Tengine-Ingress supports the standard Ingress specification based on [kubernetes/ingress-nginx](https://github.com/kubernetes/ingress-nginx) repo.


## Features

* Dynamically reconfigure the servers, locations and upstreams for Ingress, Secret, Service and Endpoint changes, without reloading or restarting worker processes.
* Support for hybrid ECC and RSA certificates for the same ingress/path.
* Dynamically reconfigure certificates and keys.
* Dynamically reconfigure anary routing based on standard and custom HTTP headers, header value, and weights.
* Dynamically reconfigure timeout setting, SSL Redirects, CORS and enabling/disabling robots for the ingress/path.
* HTTP/3 support (QUIC v1 and draft-29).
* Supports watching Ingress and Secrets in a dedicated storage k8s cluster via kubeconfig.
* Watch changes in Ingress and Secrets and do rolling upgrades for associated StatefulSet of Tengine-Ingress, without tengine reload.
* New CRD IngressCheckSum and SecretCheckSum to verify the integrity of Ingress and Secret in the cluster.


## Installation

    # ./build.sh tengine
    # ./build.sh ingress


## Documentation

The homepage of Tengine is at [http://tengine.taobao.org/](http://tengine.taobao.org/)


## Contact

[https://github.com/alibaba/tengine-ingress/issues](https://github.com/alibaba/tengine-ingress/issues)

Dingtalk user group: 23394285


## License

[Apache License 2.0](https://github.com/alibaba/tengine-ingress/blob/main/LICENSE)