
%define tengine_name      tengine
%define tengine_version   3.1.0
%define tengine_group     admin
%define tengine_home      /usr/local/tengine/
%define tengine_datadir   %{tengine_home}
%define tengine_sbindir   %{tengine_datadir}/bin
%define tengine_logdir    %{tengine_datadir}/logs
%define tengine_confdir   %{tengine_datadir}/conf
%define tengine_home_data  %{tengine_datadir}/data
%define tengine_libdir    %{tengine_datadir}/lib
%define tengine_moduledir %{tengine_datadir}/modules
%define tengine_modstodir %{tengine_datadir}/modules-%{tengine_version}-%{release}
%define tengine_incldir   %{tengine_home_data}/include

Name:           %{tengine_name}
Version:        %{tengine_version}
Release:        1%{?dist}
Packager:       weiyue <weiyue@taobao.com>
Summary:        Robust, small and high performance http and reverse proxy server
Group:          System Environment/Daemons
Source:         %{tengine_name}-%{tengine_version}.tar.gz

Source1:        jemalloc-4.0.4.tar.gz
Source2:        BabaSSL-8.3.2.tar.gz
Source3:        zlib-1.2.8.tar.gz
Source4:        luajit2-2.1-20220411.tar.gz
Source5:        pcre-8.45.tar.gz
Source6:        lua-resty-core-0.1.27.tar.gz

# BSD License (two clause)
License:        BSD
URL:            git@github.com:alibaba/tengine.git
BuildRoot:      %{_tmppath}/%{name}-%{tengine_version}-%{release}-root-%(%{__id_u} -n)

%description
Tengine is an HTTP(S) server, HTTP(S) reverse proxy and IMAP/POP3
proxy server written by Igor Sysoev.
Changes: https://tengine.taobao.org/changelog.html

%prep
%setup -q
%setup -b 1
%setup -b 2
%setup -b 3
%setup -b 4
%setup -b 5
%setup -b 6

%build

cd ../

echo "build jemalloc"
cd jemalloc-4.0.4
./autogen.sh
make -j 32
cd ../

echo "build BabaSSL"
cd Tongsuo-8.3.2
./config --prefix=/usr/local/babassl
make
SSL_TYPE_STR="babassl"
SSL_PATH_STR="${PWD}"
SSL_INC_PATH_STR="${PWD}/include"
SSL_LIB_PATH_STR="${PWD}/libssl.a;${PWD}/libcrypto.a"
cd ../

echo "build luajit"
cd luajit2-2.1-20220411
make CCDEBUG=-g
cp src/libluajit.a src/libluajit-5.1.a

export LUAJIT_LIB=${PWD}/src
export LUAJIT_INC=${PWD}/src
cd ../

cd %{tengine_name}-%{tengine_version}
./configure \
    --group=%{tengine_group} \
    --prefix=%{tengine_home} \
    --sbin-path=bin/tengine \
    --conf-path=conf/nginx-proxy.conf \
    --error-log-path=logs/error.log \
    --pid-path=logs/%{name}.pid \
    --lock-path=logs/%{name}.lock \
    --http-log-path=logs/access.log \
    --http-client-body-temp-path=data/client_body \
    --http-proxy-temp-path=data/proxy \
    --http-fastcgi-temp-path=data/fastcgi \
    --http-uwsgi-temp-path=data/uwsgi \
    --http-scgi-temp-path=data/scgi \
    --with-http_v2_module \
    --with-openssl="%_builddir/Tongsuo-8.3.2" \
    --with-http_realip_module \
    --without-select_module \
    --without-poll_module \
    --with-http_secure_link_module \
    --with-http_gzip_static_module \
    --with-zlib=%_builddir/zlib-1.2.8 \
    --with-zlib-opt='-O3 -fPIC' \
    --with-jemalloc=%_builddir/jemalloc-4.0.4 \
    --with-pcre="%_builddir/pcre-8.45" \
    --with-compat \
    --with-pcre-jit \
    --with-http_ssl_module \
    --with-http_stub_status_module \
    --with-http_sub_module \
    --with-stream \
    --with-stream_ssl_module \
    --with-stream_ssl_preread_module \
    --with-threads \
    --with-http_gunzip_module \
    --add-module=modules/ngx_http_lua_module \
    --add-module=modules/ngx_debug_pool \
    --add-module=modules/ngx_backtrace_module \
    --add-module=modules/ngx_http_sysguard_module \
    --add-module=modules/ngx_http_footer_filter_module \
    --add-module=modules/ngx_http_trim_filter_module \
    --add-module=modules/ngx_http_reqstat_module \
    --add-module=modules/ngx_http_proxy_connect_module \
    --add-module=modules/ngx_http_upstream_check_module \
    --add-module=modules/ngx_http_upstream_dyups_module \
    --add-module=modules/ngx_http_upstream_dynamic_module \
    --add-module=modules/ngx_http_upstream_session_sticky_module \
    --with-cc-opt="-fgnu89-inline -DT_HTTP_X_BODY_STREAM -fPIC -DT_RPM_VERSION=\\\"%{tengine_name}-%{tengine_version}\\\" -Wp,-U_FORTIFY_SOURCE -I modules/ngx_http_lua_module/src " \
    --with-ld-opt="-Wl,-rpath=%{tengine_libdir}"

make

%install
rm -rf %{buildroot}

mkdir -p %{buildroot}%{tengine_sbindir}/
cp objs/nginx %{buildroot}%{tengine_sbindir}/tengine

cd ../lua-resty-core-0.1.27
mkdir -p %{buildroot}%{tengine_libdir}/lua
cp -r lib/* %{buildroot}%{tengine_libdir}/lua/

%files
%dir %{tengine_sbindir}
%{tengine_sbindir}/tengine
%attr(6755, root, %{tengine_group}) %{tengine_sbindir}/tengine
%{tengine_libdir}/lua/ngx/balancer.lua
%{tengine_libdir}/lua/ngx/balancer.md
%{tengine_libdir}/lua/ngx/base64.lua
%{tengine_libdir}/lua/ngx/base64.md
%{tengine_libdir}/lua/ngx/errlog.lua
%{tengine_libdir}/lua/ngx/errlog.md
%{tengine_libdir}/lua/ngx/ocsp.lua
%{tengine_libdir}/lua/ngx/ocsp.md
%{tengine_libdir}/lua/ngx/pipe.lua
%{tengine_libdir}/lua/ngx/pipe.md
%{tengine_libdir}/lua/ngx/process.lua
%{tengine_libdir}/lua/ngx/process.md
%{tengine_libdir}/lua/ngx/re.lua
%{tengine_libdir}/lua/ngx/re.md
%{tengine_libdir}/lua/ngx/req.lua
%{tengine_libdir}/lua/ngx/req.md
%{tengine_libdir}/lua/ngx/resp.lua
%{tengine_libdir}/lua/ngx/resp.md
%{tengine_libdir}/lua/ngx/semaphore.lua
%{tengine_libdir}/lua/ngx/semaphore.md
%{tengine_libdir}/lua/ngx/ssl.lua
%{tengine_libdir}/lua/ngx/ssl.md
%{tengine_libdir}/lua/ngx/ssl/clienthello.lua
%{tengine_libdir}/lua/ngx/ssl/clienthello.md
%{tengine_libdir}/lua/ngx/ssl/session.lua
%{tengine_libdir}/lua/ngx/ssl/session.md
%{tengine_libdir}/lua/resty/core.lua
%{tengine_libdir}/lua/resty/core/base.lua
%{tengine_libdir}/lua/resty/core/base64.lua
%{tengine_libdir}/lua/resty/core/coroutine.lua
%{tengine_libdir}/lua/resty/core/ctx.lua
%{tengine_libdir}/lua/resty/core/exit.lua
%{tengine_libdir}/lua/resty/core/hash.lua
%{tengine_libdir}/lua/resty/core/misc.lua
%{tengine_libdir}/lua/resty/core/ndk.lua
%{tengine_libdir}/lua/resty/core/param.lua
%{tengine_libdir}/lua/resty/core/phase.lua
%{tengine_libdir}/lua/resty/core/regex.lua
%{tengine_libdir}/lua/resty/core/request.lua
%{tengine_libdir}/lua/resty/core/response.lua
%{tengine_libdir}/lua/resty/core/shdict.lua
%{tengine_libdir}/lua/resty/core/socket.lua
%{tengine_libdir}/lua/resty/core/time.lua
%{tengine_libdir}/lua/resty/core/time.md
%{tengine_libdir}/lua/resty/core/uri.lua
%{tengine_libdir}/lua/resty/core/utils.lua
%{tengine_libdir}/lua/resty/core/var.lua
%{tengine_libdir}/lua/resty/core/worker.lua

%changelog
