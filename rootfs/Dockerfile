# Copyright 2015 The Kubernetes Authors. All rights reserved.
# Copyright 2023 The Alibaba Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG BASE_IMAGE

FROM ${BASE_IMAGE}

ARG VERSION
ARG GOLANG_VERSION
ARG GOLANG_SHA

LABEL org.opencontainers.image.title="Tengine Ingress Controller for Kubernetes"
LABEL org.opencontainers.image.documentation="https://tengine.taobao.org/ingress/"
LABEL org.opencontainers.image.source="https://github.com/alibaba/tengine-ingress"
LABEL org.opencontainers.image.vendor="The Alibaba Authors"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.version="${VERSION}"

WORKDIR  /etc/nginx

#RUN apk add -U --no-cache \
#  diffutils \
#  libcap

COPY --chown=admin:admin . /

RUN rpm --rebuilddb && yum install -y -b current golang-bin

RUN set -eux; \
	export \
# set GOROOT_BOOTSTRAP such that we can actually build Go
		GOROOT_BOOTSTRAP="$(go env GOROOT)" \
# ... and set "cross-building" related vars to the installed system's values so that we create a build targeting the proper arch
# (for example, if our build host is GOARCH=amd64, but our build env/image is GOARCH=386, our build needs GOARCH=386)
		GOOS="$(go env GOOS)" \
		GOARCH="$(go env GOARCH)" \
		GOHOSTOS="$(go env GOHOSTOS)" \
		GOHOSTARCH="$(go env GOHOSTARCH)" \
	; \
# also explicitly set GO386 and GOARM if appropriate
# https://github.com/docker-library/golang/issues/184
#	apkArch="$(apk --print-arch)"; \
#	case "$apkArch" in \
#		armhf) export GOARM='6' ;; \
#		armv7) export GOARM='7' ;; \
#		x86) export GO386='387' ;; \
#	esac; \
#	\
	#wget -O go.tgz "https://golang.org/dl/go$GOLANG_VERSION.src.tar.gz"; \
        mv /source/go1.13.8.src.tar.gz go.tgz; \
	echo "$GOLANG_SHA *go.tgz" | sha256sum -c -; \
	tar -C /usr/local -xzf go.tgz; \
	rm go.tgz; \
	\
	cd /usr/local/go/src; \
	./make.bash; \
	\
	rm -rf \
# https://github.com/golang/go/blob/0b30cf534a03618162d3015c8705dd2231e34703/src/cmd/dist/buildtool.go#L121-L125
		/usr/local/go/pkg/bootstrap \
# https://golang.org/cl/82095
# https://github.com/golang/build/blob/e3fe1605c30f6a3fd136b561569933312ede8782/cmd/release/releaselet.go#L56
		/usr/local/go/pkg/obj \
	; \
	\
	export PATH="/usr/local/go/bin:$PATH"; \
	go version \
  ; \
  export GO111MODULE=on; \
  export GONOPROXY=""; \
  export GOPROXY="https://goproxy.io,direct"; \
  export GOPRIVATE="gitlab.alibaba-inc.com,code.alibaba-inc.com" \
  #go get github.com/go-delve/delve/cmd/dlv; \
  #go get github.com/onsi/ginkgo/ginkgo; \
  #go get golang.org/x/lint/golint \
  ;

RUN yum erase -y golang-bin golang golang-src

ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH

# Fix permission during the build to avoid issues at runtime
# with volumes (custom templates)
RUN bash -xeu -c ' \
  writeDirs=( \
    /etc/ingress-controller \
    /etc/ingress-controller/ssl \
    /etc/ingress-controller/auth \
    /var/log \
    /home/admin/tengine-ingress/logs \
    /usr/local/tengine/logs \
  ); \
  for dir in "${writeDirs[@]}"; do \
    mkdir -p ${dir}; \
    chown -R admin.admin ${dir}; \
  done'

# Create symlinks to redirect tengine logs to stdout and stderr docker log collector
RUN  ln -sf /dev/stdout /home/admin/tengine-ingress/logs/access.log \
  && ln -sf /dev/stderr /home/admin/tengine-ingress/logs/error.log

RUN chown admin:admin /home/admin/*.sh \
  && chmod 0775 /home/admin/*.sh \
  && chown admin:admin /home/admin/nginxctl \
  && chmod 0755 /home/admin/nginxctl

#ENTRYPOINT ["/usr/bin/dumb-init", "--"]

CMD ["/tengine-ingress-controller"]
