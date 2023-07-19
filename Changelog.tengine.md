# Changelog

### 1.0.0

**Image:** `tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.0.0`

- Dynamically reconfigure the servers, locations and upstreams for Ingress, Secret, Service and Endpoint changes, without reloading or restarting worker processes.
- Dynamically reconfigure canary routing based on standard and custom HTTP headers, header value, and weights.
- Dynamically reconfigure timeout setting, SSL Redirects, CORS and enabling/disabling robots for the ingress/path.
- Dynamically reconfigure certificates and keys.
- Support for hybrid ECC and RSA certificates for the same ingress/path.
- HTTP/3 support (QUIC v1 and draft-29).
- Supports watching Ingress and Secrets in a dedicated storage k8s cluster via kubeconfig.
- Watch changes in Ingress and Secrets and do rolling upgrades for associated StatefulSet of Tengine-Ingress, without tengine reload.
- New CRD IngressCheckSum and SecretCheckSum to verify the integrity of Ingress and Secret in the cluster.

_Changes:_

_Documentation:_
