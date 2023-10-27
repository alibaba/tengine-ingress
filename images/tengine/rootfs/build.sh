#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

export DEBIAN_FRONTEND=noninteractive

export TENGINE_VERSION=3.1.0
export XQUIC_VERSION=1.6.0
export XUDP_LIB_VERSION=1.0.2
export BABASSL_VERSION=8.3.2
export NDK_VERSION=0.3.1rc1
export SETMISC_VERSION=0.32
export MORE_HEADERS_VERSION=0.34
export NGINX_DIGEST_AUTH=1.0.0
export NGINX_SUBSTITUTIONS=bc58cb11844bc42735bbaef7085ea86ace46d05b
export NGINX_OPENTRACING_VERSION=0.9.0
export OPENTRACING_CPP_VERSION=1.5.1
export ZIPKIN_CPP_VERSION=0.5.2
export JAEGER_VERSION=0.4.2
export MSGPACK_VERSION=3.2.0
export DATADOG_CPP_VERSION=1.1.3
export MODSECURITY_VERSION=1.0.1
export MODSECURITY_LIB_VERSION=3.0.8
export MODSECURITY_PYTHON_BINDINGS_VERSION=bc625d5bb0bac6a64bcce8dc9902208612399348
export LIBINJECTION_VERSION=bfba51f5af8f1f6cf5d6c4bf862f1e2474e018e3
export SECRULES_LANGUAGE_TESTS_VERSION=a3d4405e5a2c90488c387e589c5534974575e35b
export OWASP_MODSECURITY_CRS_VERSION=3.2.0
export LUA_NGX_VERSION=0.10.25
export LUA_STREAM_NGX_VERSION=0.0.13
export LUA_UPSTREAM_VERSION=0.07
export LUA_BRIDGE_TRACER_VERSION=0.1.1
export NGINX_INFLUXDB_VERSION=5b09391cb7b9a889687c0aa67964c06a2d933e8b
export GEOIP2_VERSION=3.4
export LIBMAXMINDDB_VERSION=1.7.1
export RESTY_LUAROCKS_VERSION=3.1.3
export LUAJIT_VERSION=2.1-20220411
export LUAJIT_VER=2.1
export LUA_RESTY_BALANCER=0.03
export LUA_RESTY_CORE=0.1.27
export LUA_CJSON_VERSION=2.1.0.7
export LUA_RESTY_COOKIE_VERSION=766ad8c15e498850ac77f5e0265f1d3f30dc4027
export PROTOBUF_C_VERSION=1.3.1
export SSDEEP_VERSION=2.14.1
export PCRE_VERSION=8.45
export ZLIB_VERSION=1.2.8
export JEMALLOC_VERSION=4.0.4
export MIMALLOC_VERSION=2.0.6
export LUA_RESTY_UPLOAD_VERSION=0.10
export LUA_RESTY_STRING_VERSION=0.11
export LUA_RESTY_DNS_VERSION=0.21-1
export LREXLIB_PCRE_VERSION=2.7.2-1
export LUA_RESTY_LOCK_VERSION=0.08
export LUA_RESTY_IPUTILS_VERSION=0.3.0-1
export LUA_RESTY_LRUCACHE_VERSION=0.09-2
export GCC_VERSION=5.4.0
export NGX_BROTLI_VERSION=1.0.0rc
export BROTLI_SUBMODULE_VERSION=1.0.9

export BUILD_PATH=/tmp/build
export TENGINE_USER=admin
export TENGINE_GROUP=admin

WITH_XUDP="0"
WITH_XUDP_MODULE=""

WITH_BACKTRACE_MODULE="--add-module=modules/ngx_backtrace_module"

ARCH=$(uname -m)

get_src()
{
  hash="$1"
  url="$2"
  f=$(basename "$url")

  echo "Downloading $url"

  curl -sSL "$url" -o "$f"
  echo "$hash  $f" | sha256sum -c - || exit 10
  tar xzf "$f"
  rm -rf "$f"
}

get_src_local()
{
  f=$(basename "$1")
  cp "$1" "$BUILD_PATH"
  cd "$BUILD_PATH"

  echo "Local source $BUILD_PATH/$f"
  tar xzf "$f"
  rm -rf "$f"
}
LINUX_RELEASE=$1

mkdir -p /etc/nginx/
echo ${LINUX_RELEASE} > /etc/nginx/linux_release

# Add admin group and user
id admin || groupadd -f admin && useradd -m -g admin admin || adduser -D -g admin admin

# Install dependencies
if [[ $LINUX_RELEASE =~ "anolisos" ]]
then
    yum clean all
    yum install -y cmake gcc curl-devel clang llvm kernel-headers autoconf automake libtool gcc-c++ pcre-devel git unzip epel-release
    yum install -y GeoIP GeoIP-devel dumb-init
    yum install -y libnl3-devel elfutils-libelf-devel libcap-devel
    WITH_XUDP="1"
elif [[ $LINUX_RELEASE =~ "alpine" ]]
then
    sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

    apk add bash gcc clang libc-dev make automake openssl-dev pcre-dev zlib-dev linux-headers libxslt-dev gd-dev geoip-dev perl-dev libedit-dev mercurial alpine-sdk findutils curl ca-certificates patch libaio-dev openssl cmake util-linux lmdb-tools wget curl-dev libprotobuf git g++ pkgconf flex bison doxygen yajl-dev lmdb-dev libtool autoconf libxml2 libxml2-dev python3 libmaxminddb-dev bc unzip dos2unix yaml-cpp coreutils dumb-init

    WITH_BACKTRACE_MODULE=""
elif [[ $LINUX_RELEASE =~ "ubuntu" ]]
then
    chmod a+x /tmp
    apt-get clean
    apt-get update
    apt-get install -y bash gcc clang libc-dev make automake libpcre3-dev libxslt-dev libgd-dev libgeoip-dev libperl-dev libedit-dev mercurial findutils curl ca-certificates patch libaio-dev cmake util-linux wget libprotobuf-dev git g++ pkgconf flex bison doxygen libyajl-dev liblmdb-dev libtool autoconf libxml2 libxml2-dev python3 libmaxminddb-dev bc unzip dos2unix libyaml-cpp-dev coreutils
    # apt-get linux-headers
else
    echo "unkown linux release:" $LINUX_RELEASE
    exit 1
fi

mkdir -p /etc/nginx/owasp-modsecurity-crs/

# Get the GeoIP data
GEOIP_FOLDER=/etc/nginx/geoip
mkdir -p $GEOIP_FOLDER

function geoip2_get {
  wget -O $GEOIP_FOLDER/$1.tar.gz $2 || { echo "Could not download $1, exiting." ; exit 1; }
  mkdir $GEOIP_FOLDER/$1 \
    && tar xf $GEOIP_FOLDER/$1.tar.gz -C $GEOIP_FOLDER/$1 --strip-components 1 \
    && mv $GEOIP_FOLDER/$1/$1.mmdb $GEOIP_FOLDER/$1.mmdb \
    && rm -rf $GEOIP_FOLDER/$1 \
    && rm -rf $GEOIP_FOLDER/$1.tar.gz
}

mkdir --verbose -p "$BUILD_PATH"
cd "$BUILD_PATH"

get_src_local "/source/luajit2-$LUAJIT_VERSION.tar.gz"

# improve compilation times
CORES=$(($(grep -c ^processor /proc/cpuinfo) - 0))

export MAKEFLAGS=-j${CORES}
export CTEST_BUILD_FLAGS=${MAKEFLAGS}
export HUNTER_JOBS_NUMBER=${CORES}
export HUNTER_USE_CACHE_SERVERS=true

# Install luajit from openresty fork
export LUAJIT_LIB=/usr/local/lib
export LUA_LIB_DIR="$LUAJIT_LIB/lua"
export LUAJIT_INC=/usr/local/include/luajit-$LUAJIT_VER

cd "$BUILD_PATH/luajit2-$LUAJIT_VERSION"
make CCDEBUG=-g CFLAGS=-fPIC
make install

cd "$BUILD_PATH"

# build opentracing lib
get_src_local "/source/opentracing-cpp-$OPENTRACING_CPP_VERSION.tar.gz"
cd "$BUILD_PATH/opentracing-cpp-$OPENTRACING_CPP_VERSION"
mkdir .build
cd .build

cmake -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_TESTING=OFF \
      -DBUILD_SHARED_LIBS=OFF \
      -DBUILD_MOCKTRACER=OFF \
      -DBUILD_STATIC_LIBS=ON \
      -DCMAKE_POSITION_INDEPENDENT_CODE:BOOL=true \
      ..

make
make install

# install openresty-gdb-utils
cd /
get_src_local "/source/openresty-gdb-utils.tar.gz"
cat > ~/.gdbinit << EOF
directory /openresty-gdb-utils

py import sys
py sys.path.append("/openresty-gdb-utils")

source luajit20.gdb
source ngx-lua.gdb
source luajit21.py
source ngx-raw-req.py
set python print-stack full
EOF

# get brotli source and deps
cd "$BUILD_PATH"
#git clone --depth=1 https://github.com/google/ngx_brotli.git
get_src_local "/source/ngx_brotli-$NGX_BROTLI_VERSION.tar.gz"
get_src_local "/source/brotli-$BROTLI_SUBMODULE_VERSION.tar.gz"
cp -RP $BUILD_PATH/brotli-$BROTLI_SUBMODULE_VERSION/* $BUILD_PATH/ngx_brotli-$NGX_BROTLI_VERSION/deps/brotli/

# build ssdeep
cd "$BUILD_PATH"
get_src_local "/source/ssdeep-$SSDEEP_VERSION.tar.gz"
#git clone https://github.com/ssdeep-project/ssdeep
cd "ssdeep-$SSDEEP_VERSION"

./bootstrap
./configure

make
make install

# get ngx_devel_kit
get_src_local "/source/ngx_devel_kit-$NDK_VERSION.tar.gz"

# get set-misc-nginx-module
get_src_local "/source/set-misc-nginx-module-$SETMISC_VERSION.tar.gz"

# get headers-more-nginx-module
get_src_local "/source/headers-more-nginx-module-$MORE_HEADERS_VERSION.tar.gz"

# get ngx_http_substitutions_filter_module
get_src_local "/source/ngx_http_substitutions_filter_module-$NGINX_SUBSTITUTIONS.tar.gz"

# get nginx-http-auth-digest
get_src_local "/source/nginx-http-auth-digest-$NGINX_DIGEST_AUTH.tar.gz"

# get nginx-influxdb-module
get_src_local "/source/nginx-influxdb-module-$NGINX_INFLUXDB_VERSION.tar.gz"

# get nginx-opentracing
get_src_local "/source/nginx-opentracing-$NGINX_OPENTRACING_VERSION.tar.gz"

# get modsecurity-nginx
get_src_local "/source/modsecurity-nginx-v$MODSECURITY_VERSION.tar.gz"

# get ngx_http_geoip2_module
get_src_local "/source/ngx_http_geoip2_module-$GEOIP2_VERSION.tar.gz"

# get lua-nginx-module
# get_src_local "/source/lua-nginx-module-$LUA_NGX_VERSION.tar.gz"

# get stream-lua-nginx-module
get_src_local "/source/stream-lua-nginx-module-$LUA_STREAM_NGX_VERSION.tar.gz"

# get lua-upstream-nginx-module
get_src_local "/source/lua-upstream-nginx-module-$LUA_UPSTREAM_VERSION.tar.gz"

# get pcre
get_src_local "/source/pcre-$PCRE_VERSION.tar.gz"

# get zlib
get_src_local "/source/zlib-$ZLIB_VERSION.tar.gz"

# get jemalloc
get_src_local "/source/jemalloc-$JEMALLOC_VERSION.tar.gz"
cd "$BUILD_PATH/jemalloc-$JEMALLOC_VERSION"
./autogen.sh
make

# build protobuf-c
get_src_local "/source/protobuf-c-$PROTOBUF_C_VERSION.tar.gz"
cd "$BUILD_PATH/protobuf-c-$PROTOBUF_C_VERSION"
./configure --disable-protoc
make
make install
export PROTOBUF_C_INC=/usr/local/include
export PROTOBUF_C_LIB=/usr/local/lib

# build libmaxminddb library
get_src_local "/source/libmaxminddb-$LIBMAXMINDDB_VERSION.tar.gz"
cd "$BUILD_PATH/libmaxminddb-$LIBMAXMINDDB_VERSION"
./configure
make
make check
make install
# ldconfig

# build xquic with babassl
get_src_local "/source/xquic-$XQUIC_VERSION.tar.gz"
cd "$BUILD_PATH/xquic-$XQUIC_VERSION"

get_src_local "/source/BabaSSL-$BABASSL_VERSION.tar.gz"
cd "$BUILD_PATH/Tongsuo-$BABASSL_VERSION"
./config --prefix=/usr/local/babassl
make
make install
SSL_TYPE_STR="babassl"
SSL_PATH_STR="${PWD}"
SSL_INC_PATH_STR="${PWD}/include"
SSL_LIB_PATH_STR="${PWD}/libssl.a;${PWD}/libcrypto.a"

cd "$BUILD_PATH/xquic-$XQUIC_VERSION"
mkdir -p build; cd build
export CFLAGS="-Wno-dangling-pointer"
cmake -DXQC_SUPPORT_SENDMMSG_BUILD=1 -DXQC_ENABLE_BBR2=1 -DXQC_DISABLE_RENO=0 -DSSL_TYPE=${SSL_TYPE_STR} -DSSL_PATH=${SSL_PATH_STR} -DSSL_INC_PATH=${SSL_INC_PATH_STR} -DSSL_LIB_PATH=${SSL_LIB_PATH_STR} ..
make
cp "$BUILD_PATH/xquic-$XQUIC_VERSION/build/libxquic.so" /usr/local/lib/

get_src_local "/source/tengine-$TENGINE_VERSION.tar.gz"

if [[ ${WITH_XUDP} == "1" ]]; then
    # build xudp library
    get_src_local "/source/libxudp-v$XUDP_LIB_VERSION.tar.gz"
    cd "$BUILD_PATH/libxudp-v$XUDP_LIB_VERSION"
    make
    cp "$BUILD_PATH/libxudp-v$XUDP_LIB_VERSION/objs/libxudp.so.$XUDP_LIB_VERSION" /usr/local/lib/
    cd /usr/local/lib/
    ln -s -f libxudp.so.$XUDP_LIB_VERSION libxudp.so.1
    ln -s -f libxudp.so.$XUDP_LIB_VERSION libxudp.so

    # build xquic-xdp
    cd "$BUILD_PATH/tengine-$TENGINE_VERSION/modules/mod_xudp/xquic-xdp"
    make config root="$BUILD_PATH/libxudp-v$XUDP_LIB_VERSION"
    make
    mkdir -p /usr/local/lib64/xquic_xdp/
    cp kern_xquic.o /usr/local/lib64/xquic_xdp/kern_xquic.o
    WITH_XUDP_MODULE="--with-xudp-inc=$BUILD_PATH/libxudp-v$XUDP_LIB_VERSION/objs \
        --with-xudp-lib=$BUILD_PATH/libxudp-v$XUDP_LIB_VERSION/objs \
        --with-xquic_xdp-inc=$BUILD_PATH/tengine-$TENGINE_VERSION/modules/mod_xudp/xquic-xdp \
        --add-module=modules/mod_xudp"
fi

# build modsecurity library
cd "$BUILD_PATH"
get_src_local "/source/modsecurity-v$MODSECURITY_LIB_VERSION.tar.gz"
get_src_local "/source/modsecurity-python-bindings-$MODSECURITY_PYTHON_BINDINGS_VERSION.tar.gz"
cp -RP $BUILD_PATH/modsecurity-python-bindings-$MODSECURITY_PYTHON_BINDINGS_VERSION/* $BUILD_PATH/modsecurity-v$MODSECURITY_LIB_VERSION/bindings/python/
get_src_local "/source/libinjection-$LIBINJECTION_VERSION.tar.gz"
cp -RP $BUILD_PATH/libinjection-$LIBINJECTION_VERSION/* $BUILD_PATH/modsecurity-v$MODSECURITY_LIB_VERSION/others/libinjection/
get_src_local "/source/secrules-language-tests-$SECRULES_LANGUAGE_TESTS_VERSION.tar.gz"
cp -RP $BUILD_PATH/secrules-language-tests-$SECRULES_LANGUAGE_TESTS_VERSION/* $BUILD_PATH/modsecurity-v$MODSECURITY_LIB_VERSION/test/test-cases/secrules-language-tests/
cd "$BUILD_PATH/modsecurity-v$MODSECURITY_LIB_VERSION"
sh build.sh

# https://github.com/SpiderLabs/ModSecurity/issues/1909#issuecomment-465926762
sed -i '115i LUA_CFLAGS="${LUA_CFLAGS} -DWITH_LUA_JIT_2_1"' build/lua.m4
sed -i '117i AC_SUBST(LUA_CFLAGS)' build/lua.m4

./configure \
  --disable-doxygen-doc \
  --disable-doxygen-html \
  --disable-examples

make
make install

mkdir -p /etc/nginx/modsecurity
cp modsecurity.conf-recommended /etc/nginx/modsecurity/modsecurity.conf
cp unicode.mapping /etc/nginx/modsecurity/unicode.mapping

# Replace serial logging with concurrent
sed -i 's|SecAuditLogType Serial|SecAuditLogType Concurrent|g' /etc/nginx/modsecurity/modsecurity.conf

# Concurrent logging implies the log is stored in several files
echo "SecAuditLogStorageDir /var/log/audit/" >> /etc/nginx/modsecurity/modsecurity.conf

# Download owasp modsecurity crs
cd /etc/nginx/

get_src_local "/source/coreruleset-$OWASP_MODSECURITY_CRS_VERSION.tar.gz"
mv coreruleset-$OWASP_MODSECURITY_CRS_VERSION owasp-modsecurity-crs
cd owasp-modsecurity-crs

mv crs-setup.conf.example crs-setup.conf
mv rules/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf.example rules/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf
mv rules/RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf.example rules/RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf
cd ..

# OWASP CRS v3 rules
mkdir -p /etc/nginx/owasp-modsecurity-crs
echo "
Include /etc/nginx/owasp-modsecurity-crs/crs-setup.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-901-INITIALIZATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-903.9001-DRUPAL-EXCLUSION-RULES.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-903.9002-WORDPRESS-EXCLUSION-RULES.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-905-COMMON-EXCEPTIONS.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-910-IP-REPUTATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-911-METHOD-ENFORCEMENT.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-912-DOS-PROTECTION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-913-SCANNER-DETECTION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-920-PROTOCOL-ENFORCEMENT.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-921-PROTOCOL-ATTACK.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-922-MULTIPART-ATTACK.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-930-APPLICATION-ATTACK-LFI.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-931-APPLICATION-ATTACK-RFI.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-932-APPLICATION-ATTACK-RCE.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-933-APPLICATION-ATTACK-PHP.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-934-APPLICATION-ATTACK-NODEJS.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-941-APPLICATION-ATTACK-XSS.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-942-APPLICATION-ATTACK-SQLI.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-943-APPLICATION-ATTACK-SESSION-FIXATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-944-APPLICATION-ATTACK-JAVA.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/REQUEST-949-BLOCKING-EVALUATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-950-DATA-LEAKAGES.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-951-DATA-LEAKAGES-SQL.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-952-DATA-LEAKAGES-JAVA.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-953-DATA-LEAKAGES-PHP.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-954-DATA-LEAKAGES-IIS.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-959-BLOCKING-EVALUATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-980-CORRELATION.conf
Include /etc/nginx/owasp-modsecurity-crs/rules/RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf
" > /etc/nginx/owasp-modsecurity-crs/nginx-modsecurity.conf

## build tengine
cd "$BUILD_PATH/tengine-$TENGINE_VERSION"

WITH_FLAGS="--with-debug \
            --with-compat \
            --with-pcre-jit \
            --with-http_ssl_module \
            --with-http_stub_status_module \
            --with-http_realip_module \
            --with-http_auth_request_module \
            --with-http_addition_module \
            --with-http_dav_module \
            --with-http_geoip_module=dynamic \
            --with-http_gzip_static_module \
            --with-http_sub_module \
            --with-http_v2_module \
            --with-stream \
            --with-stream_ssl_module \
            --with-stream_ssl_preread_module \
            --with-threads \
            --with-http_secure_link_module \
            --with-http_gunzip_module"

## "Combining -flto with -g is currently experimental and expected to produce unexpected results."
## https://gcc.gnu.org/onlinedocs/gcc/Optimize-Options.html
CC_OPT="-g -Og -fstack-protector-strong \
  -DT_HTTP_X_BODY_STREAM \
  -Wformat \
  -Werror=format-security \
  -Wno-deprecated-declarations \
  -fno-strict-aliasing \
  -D_FORTIFY_SOURCE=2 \
  --param=ssp-buffer-size=4 \
  -DTCP_FASTOPEN=23 \
  -fPIC \
  -Wp,-U_FORTIFY_SOURCE -I modules/ngx_http_lua_module/src"

LD_OPT="-fPIC -Wl,-z,relro -Wl,-z,now,-rpath=/usr/local/lib/ -L/usr/local/lib/ -lm"

if [[ ${ARCH} != "aarch64" ]]; then
  WITH_FLAGS="${WITH_FLAGS} --with-file-aio"
fi

if [[ ${ARCH} == "x86_64" ]]; then
  CC_OPT="${CC_OPT} -m64 -mtune=native"
fi

WITH_MODULES="--add-module=modules/ngx_http_lua_module \
  --add-module=$BUILD_PATH/stream-lua-nginx-module-$LUA_STREAM_NGX_VERSION \
  --add-module=$BUILD_PATH/lua-upstream-nginx-module-$LUA_UPSTREAM_VERSION \
  --add-module=$BUILD_PATH/ngx_devel_kit-$NDK_VERSION \
  --add-module=$BUILD_PATH/set-misc-nginx-module-$SETMISC_VERSION \
  --add-module=$BUILD_PATH/headers-more-nginx-module-$MORE_HEADERS_VERSION \
  --add-module=$BUILD_PATH/ngx_http_substitutions_filter_module-$NGINX_SUBSTITUTIONS \
  --add-dynamic-module=$BUILD_PATH/nginx-http-auth-digest-$NGINX_DIGEST_AUTH \
  --add-dynamic-module=$BUILD_PATH/nginx-influxdb-module-$NGINX_INFLUXDB_VERSION \
  --add-dynamic-module=$BUILD_PATH/nginx-opentracing-$NGINX_OPENTRACING_VERSION/opentracing \
  --add-dynamic-module=$BUILD_PATH/modsecurity-nginx-v$MODSECURITY_VERSION \
  --add-dynamic-module=$BUILD_PATH/ngx_http_geoip2_module-$GEOIP2_VERSION \
  --add-dynamic-module=$BUILD_PATH/ngx_brotli-$NGX_BROTLI_VERSION"

./configure \
  --prefix=/usr/local/tengine \
  --sbin-path=sbin/tengine \
  --conf-path=/etc/nginx/nginx.conf \
  --modules-path=/etc/nginx/modules \
  --http-log-path=/home/admin/tengine-ingress/logs/access.log \
  --error-log-path=/home/admin/tengine-ingress/logs/error.log \
  --lock-path=/var/lock/nginx.lock \
  --pid-path=/run/nginx.pid \
  --http-client-body-temp-path=/var/lib/nginx/body \
  --http-fastcgi-temp-path=/var/lib/nginx/fastcgi \
  --http-proxy-temp-path=/var/lib/nginx/proxy \
  --http-scgi-temp-path=/var/lib/nginx/scgi \
  --http-uwsgi-temp-path=/var/lib/nginx/uwsgi \
  ${WITH_FLAGS} \
  --with-pcre="$BUILD_PATH/pcre-$PCRE_VERSION" \
  --with-pcre-opt=-fPIC \
  --with-xquic-inc="$BUILD_PATH/xquic-$XQUIC_VERSION/include" \
  --with-xquic-lib="$BUILD_PATH/xquic-$XQUIC_VERSION/build" \
  --without-select_module \
  --without-poll_module \
  --without-mail_pop3_module \
  --without-mail_smtp_module \
  --without-mail_imap_module \
  --without-http_uwsgi_module \
  --without-http_scgi_module \
  --with-openssl="$BUILD_PATH/Tongsuo-$BABASSL_VERSION" \
  --with-zlib="$BUILD_PATH/zlib-$ZLIB_VERSION" \
  --with-zlib-opt='-O3 -fPIC' \
  --with-jemalloc="$BUILD_PATH/jemalloc-$JEMALLOC_VERSION" \
  --add-module=modules/ngx_debug_pool \
  --add-module=modules/mod_common \
  --add-module=modules/mod_strategy \
  ${WITH_BACKTRACE_MODULE} \
  --add-module=modules/ngx_http_xquic_module \
  ${WITH_XUDP_MODULE} \
  --add-module=modules/ngx_http_sysguard_module \
  --add-module=modules/ngx_http_footer_filter_module \
  --add-module=modules/ngx_http_trim_filter_module \
  --add-module=modules/ngx_http_reqstat_module \
  --add-module=modules/ngx_http_proxy_connect_module \
  --add-module=modules/ngx_http_upstream_check_module \
  --add-module=modules/ngx_http_upstream_dyups_module \
  --add-module=modules/ngx_http_upstream_dynamic_module \
  --add-module=modules/ngx_http_upstream_session_sticky_module \
  --add-module=modules/ngx_ingress_module \
  --user=${TENGINE_USER} \
  --group=${TENGINE_GROUP} \
  --with-cc-opt="${CC_OPT}" \
  --with-ld-opt="${LD_OPT}" \
  ${WITH_MODULES}

make
make install

get_src_local "/source/luarocks-${RESTY_LUAROCKS_VERSION}.tar.gz"
cd "$BUILD_PATH/luarocks-${RESTY_LUAROCKS_VERSION}"
./configure \
  --lua-suffix=jit-2.1.0-beta3 \
  --with-lua-include=/usr/local/include/luajit-$LUAJIT_VER

make
make install

if [[ ${ARCH} != "armv7l" ]]; then
  luarocks install lrexlib-pcre $LREXLIB_PCRE_VERSION
fi

export LUA_INCLUDE_DIR=/usr/local/include/luajit-$LUAJIT_VER

ln -s $LUA_INCLUDE_DIR /usr/include/lua5.1

get_src_local "/source/lua-resty-core-$LUA_RESTY_CORE.tar.gz"
cd "$BUILD_PATH/lua-resty-core-$LUA_RESTY_CORE"
make install

get_src_local "/source/lua-resty-balancer-$LUA_RESTY_BALANCER.tar.gz"
cd "$BUILD_PATH/lua-resty-balancer-$LUA_RESTY_BALANCER"
make all
make install

get_src_local "/source/lua-cjson-$LUA_CJSON_VERSION.tar.gz"
cd "$BUILD_PATH/lua-cjson-$LUA_CJSON_VERSION"
make all
make install

get_src_local "/source/lua-resty-cookie-$LUA_RESTY_COOKIE_VERSION.tar.gz"
cd "$BUILD_PATH/lua-resty-cookie-$LUA_RESTY_COOKIE_VERSION"
make all
make install

echo -e '[url "https://github.com/"]\n  insteadOf = "git://github.com/"' >> ~/.gitconfig

luarocks install lua-resty-iputils $LUA_RESTY_IPUTILS_VERSION
luarocks install lua-resty-lrucache $LUA_RESTY_LRUCACHE_VERSION

#luarocks install lua-resty-lock
get_src_local "/source/lua-resty-lock-$LUA_RESTY_LOCK_VERSION.tar.gz"
cd "$BUILD_PATH/lua-resty-lock-$LUA_RESTY_LOCK_VERSION"
make all
make install

luarocks install lua-resty-dns $LUA_RESTY_DNS_VERSION

# required for OCSP verification
luarocks install lua-resty-http

get_src_local "/source/lua-resty-upload-$LUA_RESTY_UPLOAD_VERSION.tar.gz"
cd "$BUILD_PATH/lua-resty-upload-$LUA_RESTY_UPLOAD_VERSION"
make install

get_src_local "/source/lua-resty-string-$LUA_RESTY_STRING_VERSION.tar.gz"
cd "$BUILD_PATH/lua-resty-string-$LUA_RESTY_STRING_VERSION"
make install

# mimalloc
cd "$BUILD_PATH"
get_src_local "/source/mimalloc-$MIMALLOC_VERSION.tar.gz"
#git clone https://github.com/microsoft/mimalloc
cd "mimalloc-$MIMALLOC_VERSION"
mkdir -p out/release
cd out/release
cmake ../..
make
make install

# update image permissions
while IFS= read -r dir
do
    mkdir -p ${dir};
    chown -R ${TENGINE_USER}:${TENGINE_GROUP} ${dir};
done <<- END
/etc/nginx
/usr/local/tengine
/opt/modsecurity/var/log
/opt/modsecurity/var/upload
/opt/modsecurity/var/audit
/var/log/audit
END

rm -rf /etc/nginx/owasp-modsecurity-crs/.git
rm -rf /etc/nginx/owasp-modsecurity-crs/util/regression-tests
rm -rf /usr/local/modsecurity/lib/libmodsecurity.a
