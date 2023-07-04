#!/bin/bash

echo "starting..."

if [[ "x$1" = "x" ]] && [[ "x$2" = "x" ]]; then
    echo "exit starting..."
    exit 0
fi

sudo /home/admin/core.sh -add
sudo chmod 6755 /usr/local/tengine/sbin/tengine

/usr/bin/dumb-init -- /tengine-ingress-controller --configmap=$1/${2}-nginx-configuration --tcp-services-configmap=$1/${2}-tcp-services --udp-services-configmap=$1/${2}-udp-services --annotations-prefix=nginx.ingress.kubernetes.io --v=${log_level} --kubeconfig=${ing_kubeconfig} --watch-namespace=${watch_namespace} --ingress-class=${ingress_class} &
