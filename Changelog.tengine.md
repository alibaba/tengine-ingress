# Changelog

### 1.1.0

**Image:** `tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.1.0`

_New Features:_

- Dynamically configure different TLS protocols for different server names without tengine reload @lianglli
- Dynamically configure multiple default TLS certificates for client-hello without SNI @lianglli  
- Supports IngressClass @lianglli
- Dynamically configure canary routing based on multiple values of a specific header, cookie or query parameter without tengine reload  @lianglli
- Dynamically configure canary routing based on the modulo operation for a specific header, cookie or query parameter without tengine reload @lianglli
- Dynamically configure canary routing to add/append custom headers or add query parameter to the HTTP request without tengine reload   @lianglli 
- Dynamically configure canary routing to add custom headers to the HTTP response without tengine reload @lianglli 
- Supports total weight of canary ingress @lianglli 
- Supports multiple CORS origins @lianglli 
- Supports 'user' config of tengine worker processes @lianglli 
- Supports watch changes in Ingress/Secret and do rolling upgrades in one time @lianglli 

_Changes:_

- Remove unnecessary and duplicate location from tengine template @lianglli 
- Update obsolete and removed APIs of Go @lianglli 
- Stopping Tengine process with layer4 LB gracefully @lianglli 

_Bugs:_

- The /configuration/certs?hostname=_ return 500 @drawing
- Duplicate location robots.txt and unknown variable "https_use_timing" in static config mode @lianglli 
- Configmap config "use-ingress-storage-cluster" is not working @lianglli 
- HTTP routes with static config mode is not working @lianglli 
- Dynamically reconfigure CORS for the ingress/path is not working @lianglli 

### 1.0.0

**Image:** `tengine-ingress-registry.cn-hangzhou.cr.aliyuncs.com/tengine/tengine-ingress:1.0.0`

- Dynamically configure the servers, locations and upstreams for Ingress, Secret, Service and Endpoint changes, without reloading or restarting worker processes. @lianglli
- Dynamically configure canary routing based on standard and custom HTTP headers, header value, and weights. @lianglli
- Dynamically configure timeout setting, SSL Redirects, CORS and enabling/disabling robots for the ingress/path. @lianglli
- Dynamically configure certificates and keys. @lianglli
- Support for hybrid ECC and RSA certificates for the same ingress/path. @lianglli
- HTTP/3 support (QUIC v1 and draft-29). @lianglli
- Supports watching Ingress and Secrets in a dedicated storage k8s cluster via kubeconfig. @lianglli
- Watch changes in Ingress and Secrets and do rolling upgrades for associated StatefulSet of Tengine-Ingress, without tengine reload. @lianglli
- New CRD IngressCheckSum and SecretCheckSum to verify the integrity of Ingress and Secret in the cluster. @lianglli
