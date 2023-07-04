# Custom Configuration

Using a [ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/) is possible to customize the NGINX configuration

For example, if we want to change the timeouts we need to create a ConfigMap:

```
$ cat configmap.yaml
apiVersion: v1
data:
  proxy-connect-timeout: "10"
  proxy-read-timeout: "120"
  proxy-send-timeout: "120"
kind: ConfigMap
metadata:
  name: nginx-configuration
```

```
curl https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/docs/examples/customization/custom-configuration/configmap.yaml \
    | kubectl apply -f -
```

If the Configmap it is updated, NGINX will be reloaded with the new configuration.
