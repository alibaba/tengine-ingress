#!/bin/sh

self_path=$(cd "$(dirname $0)";pwd)
self_parent=$(dirname $self_path)

# prepare
export GOPATH=$(pwd|awk -F '/src/' '{ print $1 }')
rm -f $self_parent/ingress-nginx
ln -s $self_path $self_parent/ingress-nginx
export DOCKER_CLI_EXPERIMENTAL=enabled

# build prepare
function build_prepare()
{
    yum-config-manager --add-repo http://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo
    ip link add name docker0 type bridge && sudo ip addr add dev docker0 192.168.5.1/24
    yum install -y https://cbs.centos.org/kojifiles/packages/container-selinux/2.84/2.el7/noarch/container-selinux-2.84-2.el7.noarch.rpm
    yum install docker-ce-19.03.8-3.el7
    systemctl enable docker.service && sudo systemctl start docker
}

# build tengine images
function build_tengine_images()
{
    cd images/tengine/

    make container || exit 1
    echo build tengine base images success

    cd ../../
}

# build e2e tengine images
function build_e2e_images()
{
    cd images/e2e-tengine/

    make || exit 1
    echo build e2e for tengine base images success

    cd ../../
}

# build ingress controller images
function build_ingress_images()
{
    make build container || exit 1
    echo build tengine ingress controller success
}

function push_ingress_image()
{
    docker push reg.docker.alibaba-inc.com/ingress-tengine/tengine-ingress-controller-amd64:0.0.1
}

function push_tengine_image()
{
    docker push reg.docker.alibaba-inc.com/ingress-tengine/tengine-amd64:0.0.1
}

function build_ingress()
{
    DIND_TASKS=1 make build || exit 1
    echo build ingress success
}

#main
case $1 in
    tengine )
        build_tengine_images
        exit
        ;;
    ingress )
        build_ingress_images
        exit
        ;;
    prepare )
        build_prepare
        ;;
    e2e )
        build_e2e_images
        ;;
    push )
        push_tengine_image
        push_ingress_image
        ;;
    build )
        build_ingress
        ;;
    * )
        build_prepare
        build_tengine_images
        build_ingress_images
        ;;
esac
