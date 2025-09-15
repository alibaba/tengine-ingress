
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
    ${WITH_FLAGS} \
    ${notinarm_flags} \
    --with-http_v2_module \
    --with-openssl="%_builddir/Tongsuo-$BABASSL_VERSION" \
    --with-http_realip_module \
    --without-select_module \
    --without-poll_module \
    --with-http_secure_link_module \
    --with-http_gzip_static_module \
    --with-zlib=%_builddir/zlib-1.2.8 \
    --with-zlib-opt='-O3 -fPIC' \
    --with-jemalloc=%_builddir/jemalloc-4.0.4 \
    --add-module=modules/ngx_http_lua_module \
    --add-module=modules/ngx_debug_pool \
    --add-module=modules/mod_common \
    --add-module=modules/mod_strategy \
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
    --add-module=modules/ngx_ingress_module \
    --with-cc-opt="-fgnu89-inline -DT_HTTP_X_BODY_STREAM -fPIC %{optflags} $(pcre-config --cflags) -DT_RPM_VERSION=\\\"%{tengine_name}-%{tengine_version}\\\" -Wp,-U_FORTIFY_SOURCE -I modules/ngx_http_lua_module/src " \
    --with-ld-opt="-Wl,-rpath=%{tengine_libdir}"

make %{?_smp_mflags}

%install
rm -rf %{buildroot}

# collect rdb dep
mkdir -p %{buildroot}%{tengine_libdir}/rdb/
cd libs/rdb
%ifnarch aarch64
%if 0%{?rhel} < 8
./build.sh
cp ngx_rdb.so %{buildroot}%{tengine_libdir}/rdb/
%endif
%endif
cd ../../

# TODO support jansson #
####cd libs/jansson-2.6
####make install
####cd ..

# TODO support sec #
####cd flatcc-0.5.1/build/install
####make install
####cd ../../..

# TODO support lua lib #
cd libs/luajit2-2.1-20250117
make
make install PREFIX=%{buildroot}/luajit
cd ../..

mkdir -p %{buildroot}%{tengine_libdir}/lua_clib
####mkdir -p %{buildroot}%{tengine_confdir}/gray_conf/lua_clib
####
cd libs/lua-clib
####if [ `uname -r  | grep -w 'aarch64'` ]; then
####aarch64-redhat-linux-gcc-9 -o lua-dir.so -shared lua-dir.c -I%{buildroot}/luajit/include/luajit-2.1 -Wall -O2 -fPIC
####else
gcc -o lua-dir.so -shared lua-dir.c -I%{buildroot}/luajit/include/luajit-2.1 -Wall -O2 -fPIC
####fi
cp lua-dir.so %{buildroot}%{tengine_libdir}/lua_clib

cd ../lua
make linux
make install INSTALL_TOP=%{buildroot}/lua
cd ..


%ifnarch aarch64
luaposix="luaposix"
%else
tar -zxvf luaposix-5.1.28.tar.gz
luaposix="luaposix-release-v5.1.28"
%endif
cd $luaposix
export LUA='%{buildroot}/lua/bin/lua'
CPPFLAGS='-I%{buildroot}/lua/include/' ./configure
make
cp .libs/posix_c.so %{buildroot}%{tengine_libdir}/lua_clib

cd ../lua-zlib-master
make clean
cmake -DUSE_LUAJIT=on -DLUA_INCLUDE_DIR=%{buildroot}/luajit/include/luajit-2.1 -DLUA_LIBRARIES=%{buildroot}/luajit/lib
make
cp zlib.so %{buildroot}%{tengine_libdir}/lua_clib

cd ../lua-cjson
cc -c -O3 -Wall -pedantic -DNDEBUG  -I%{buildroot}/luajit/include/luajit-2.1 -fpic -o lua_cjson.o lua_cjson.c
cc -c -O3 -Wall -pedantic -DNDEBUG  -I%{buildroot}/luajit/include/luajit-2.1 -fpic -o strbuf.o strbuf.c
cc -c -O3 -Wall -pedantic -DNDEBUG  -I%{buildroot}/luajit/include/luajit-2.1 -fpic -o fpconv.o fpconv.c
cc  -shared -o cjson.so lua_cjson.o strbuf.o fpconv.o
mkdir -p %{buildroot}%{tengine_confdir}/tair_rest/lua_clib
cp cjson.so %{buildroot}%{tengine_libdir}/lua_clib
####
####%if %{with liaoyuan}
####if [ -f ../liaoyuan-client4tengine/install.sh ];then
####cd ../liaoyuan-client4tengine
####./install.sh %{buildroot}%{tengine_sbindir} %{buildroot}%{tengine_libdir}/liaoyuan
####fi
####%endif
####
cd ../../

make install DESTDIR=%{buildroot} INSTALLDIRS=vendor
mkdir -p %{buildroot}%{tengine_modstodir}
# support dso #
mv %{buildroot}%{tengine_moduledir}/* %{buildroot}%{tengine_modstodir}

# TODO: support luajit #
####export LUAJIT_LIB=%{buildroot}/luajit/lib
####export LUAJIT_INC=%{buildroot}/luajit/include/luajit-2.1

now_path=$(pwd)/modules
libxquic_path=$(pwd)/libs/xquic

sp=$(echo %{tengine_datadir} | sed -e 's/\//\\\//g')
tp=$(echo %{buildroot}%{tengine_datadir} | sed -e 's/\//\\\//g')

cp -r /usr/local/babassl/include/* %{buildroot}%{tengine_datadir}/data/include/

####cp -r /usr/local/ali-openssl/include/* %{buildroot}%{tengine_datadir}/data/include/

# TODO: support lua json #
####cp -r %{buildroot}/jansson/include/* %{buildroot}%{tengine_datadir}/data/include/
####cp -r %{buildroot}/luajit/include/luajit-2.1/* %{buildroot}%{tengine_datadir}/data/include/

mkdir -p %{buildroot}%{tengine_modstodir}/tmd4
# TODO: support dso #
####cp %{buildroot}%{tengine_sbindir}/dso-tool %{buildroot}%{tengine_sbindir}/dso-tool-local
####sed -i -e "s/${sp}/${tp}/g" %{buildroot}%{tengine_sbindir}/dso-tool-local
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_tmd/tmd_3.0.4/http
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_tmd/tmd_3.0.4/proc
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir}/tmd4 -a=$now_path/mod_tmd/tmd_4.0.0
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_beacon
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_subs
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_substitutions
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_gunzip
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_bucket_id
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_scroll_log
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_cdn_variables
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_req_auth
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_redis2_nginx
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/ngx_http_log_if_module
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_set_misc
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_cookie_pass
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_srcache
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_tbip  --with-ld-opt='-L /usr/local/c_tbip/lib -ltbip_api'
####%{buildroot}%{tengine_sbindir}/dso-tool-local --with-ld-opt=-lstdc++ -d=%{buildroot}%{tengine_modstodir} -a=$now_path/mod_hsf
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_beacon/alibeacon
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_alicookie
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_hummock/src/client
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_img_subs
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_style_combine/src/client
####%{buildroot}%{tengine_sbindir}/dso-tool-local -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_style_combine/src/nginx
####%{buildroot}%{tengine_sbindir}/dso-tool-local --with-ld-opt='-L./ -lotsclient -lprotobuf_static -lcurl -lidn -lrt' -d=%{buildroot}%{tengine_modstodir} --add-module=$now_path/mod_odps_tunnel_uri_router
####
####rm %{buildroot}%{tengine_sbindir}/dso-tool-local
rm -rf %{buildroot}/jansson
rm -rf %{buildroot}/flatcc
rm -rf %{buildroot}/luajit
rm -rf %{buildroot}/lua

find %{buildroot} -type f -name .packlist -exec rm -f {} \;
find %{buildroot} -type f -name perllocal.pod -exec rm -f {} \;
find %{buildroot} -type f -empty -exec rm -f {} \;
find %{buildroot} -type d -name html -print | xargs rm -rf
find %{buildroot} -type f -exec chmod 0644 {} \;
find %{buildroot} -type f -name '*.so' -exec chmod 0755 {} \;
chmod 0755 %{buildroot}%{tengine_sbindir}/tengine

%{__install} -p -D -m 0644 %{SOURCE1} %{buildroot}%{tengine_confdir}/nginx-proxy.conf
%{__install} -p -D -m 0644 %{SOURCE3} %{buildroot}%{tengine_confdir}/ip.dat
%{__install} -p -D -m 0644 %{SOURCE4} %{buildroot}%{tengine_confdir}/sm2tr.txt
%{__install} -p -d -m 0755 %{buildroot}%{tengine_libdir}/lua_lib
####%{__install} -p -D -m 0644 %{SOURCE29} %{buildroot}%{tengine_libdir}/lua_lib/nginx-admin-utils.lua
%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/admin
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/admin/lua_script
####%{__install} -p -D -m 0644 %{SOURCE28} %{buildroot}%{tengine_confdir}/admin/lua_script/nginx-admin-action.lua
%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/apps
# TODO suppprt gray #
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_strategy
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/lua_lib
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/lua_clib
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/lua_script
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/backup
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/test
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/json
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/resource
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/gray_conf/error_confs
####%{__install} -p -D -m 0644 %{SOURCE8} %{buildroot}%{tengine_confdir}/gray_conf/lua_script/nginx-gray.lua
####%{__install} -p -D -m 0644 %{SOURCE9} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/json.lua
####%{__install} -p -D -m 0644 %{SOURCE12} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_taobao.conf
####%{__install} -p -D -m 0644 %{SOURCE13} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_beijing.conf
####%{__install} -p -D -m 0644 %{SOURCE14} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_hangzhou.conf
####%{__install} -p -D -m 0644 %{SOURCE15} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_guangzhou.conf
####%{__install} -p -D -m 0644 %{SOURCE16} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_shanghai.conf
####%{__install} -p -D -m 0644 %{SOURCE17} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_shenzhen.conf
####%{__install} -p -D -m 0644 %{SOURCE40} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_guizhou.conf
####%{__install} -p -D -m 0644 %{SOURCE41} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_hubei.conf
####%{__install} -p -D -m 0644 %{SOURCE42} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_jiangsu.conf
####%{__install} -p -D -m 0644 %{SOURCE43} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_tianjin.conf
####%{__install} -p -D -m 0644 %{SOURCE44} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_yunnan.conf
####%{__install} -p -D -m 0644 %{SOURCE45} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_henan.conf
####%{__install} -p -D -m 0644 %{SOURCE46} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_hunan.conf
####%{__install} -p -D -m 0644 %{SOURCE47} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_jilin.conf
####%{__install} -p -D -m 0644 %{SOURCE48} %{buildroot}%{tengine_confdir}/gray_conf/resource/geo_zhejiang.conf
####%{__install} -p -D -m 0644 %{SOURCE7} %{buildroot}%{tengine_confdir}/gray_conf/resource/gray-config-server.conf
%{__install} -p -d -m 0755 %{buildroot}%{tengine_home_data}
%{__install} -p -d -m 0755 %{buildroot}%{tengine_logdir}
%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/tair_rest
%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/tair_rest/lua_lib
%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/tair_rest/lua_clib
%{__install} -p -D -m 0644 %{SOURCE21} %{buildroot}%{tengine_confdir}/tair_rest/lua_lib/tair.lua
%{__install} -p -D -m 0755 %{SOURCE2} %{buildroot}%{tengine_sbindir}/nginxctl
%{__install} -p -D -m 0755 %{SOURCE81} %{buildroot}%{tengine_sbindir}/nginx-mem.sh
%{__install} -p -D -m 0755 %{SOURCE30} %{buildroot}%{tengine_sbindir}/nginx-admin-ctl
%{__install} -p -D -m 0755 %{SOURCE80} %{buildroot}%{tengine_sbindir}/reqstatus.py
%{__install} -p -D -m 0644 %{SOURCE27} %{buildroot}%{tengine_confdir}/nginx-admin.conf
####%{__install} -p -D -m 0644 %{SOURCE31} %{buildroot}%{tengine_confdir}/gray_conf/lua_script/gray_admin.lua
####%{__install} -p -D -m 0644 %{SOURCE32} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/gray_conf.lua
####%{__install} -p -D -m 0644 %{SOURCE33} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/gray_utils.lua
####%{__install} -p -D -m 0644 %{SOURCE34} %{buildroot}%{tengine_confdir}/gray_conf/lua_script/gray_engine.lua
####%{__install} -p -D -m 0644 %{SOURCE35} %{buildroot}%{tengine_confdir}/gray_conf/lua_script/engine_init.lua
####%{__install} -p -D -m 0644 %{SOURCE36} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/engine_conf_creator.lua
####%{__install} -p -D -m 0644 %{SOURCE37} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/tengine_conf_parser.lua
####%{__install} -p -D -m 0644 %{SOURCE38} %{buildroot}%{tengine_confdir}/gray_conf/lua_lib/tengine_operator.lua
####%{__install} -p -D -m 0644 %{SOURCE39} %{buildroot}%{tengine_confdir}/gray_conf/resource/gray-log-format.conf
%{__install} -p -D -m 0644 %{SOURCE50} %{buildroot}%{tengine_confdir}/cell_main.conf
%{__install} -p -D -m 0644 %{SOURCE51} %{buildroot}%{tengine_confdir}/cell_server.conf
%{__install} -p -D -m 0644 %{SOURCE52} %{buildroot}%{tengine_confdir}/set_user_unit.lua
%{__install} -p -D -m 0644 %{SOURCE63} %{buildroot}%{tengine_confdir}/med_srv.conf
%{__install} -p -D -m 0644 %{SOURCE64} %{buildroot}%{tengine_confdir}/med_http.conf
%{__install} -p -D -m 0755 %{SOURCE73} %{buildroot}%{tengine_sbindir}/setup_services.sh
%{__install} -p -D -m 0644 %{SOURCE70} %{buildroot}%{tengine_confdir}/dso.conf
%{__install} -p -D -m 0644 %{SOURCE71} %{buildroot}%{tengine_confdir}/user.conf
%{__install} -p -D -m 0644 %{SOURCE72} %{buildroot}%{tengine_confdir}/services.conf

#detector
%{__install} -p -D -m 0644 %{SOURCE74} %{buildroot}%{tengine_confdir}/med.js
%{__install} -p -D -m 0644 %{SOURCE75} %{buildroot}%{tengine_confdir}/detector.conf
%{__install} -p -D -m 0644 %{SOURCE76} %{buildroot}%{tengine_confdir}/detector_srv.conf

# TODO support sec #
%{__install} -p -D -m 0644 %{SOURCE89} %{buildroot}%{tengine_confdir}/sec_http.conf
%{__install} -p -D -m 0644 %{SOURCE90} %{buildroot}%{tengine_confdir}/sec_loc.conf
%{__install} -p -D -m 0644 %{SOURCE91} %{buildroot}%{tengine_confdir}/sec_off.conf
%{__install} -p -D -m 0644 %{SOURCE92} %{buildroot}%{tengine_confdir}/sinfo.conf

#xquic
%{__install} -p -D -m 0755 $libxquic_path/build/libxquic.so %{buildroot}%{tengine_libdir}/libxquic.so

#####tmd conf: S22-S26, S54-S62
####%define tmd_conf_dir    %{tengine_home_data}/tmd-%{tengine_version}-%{release}
####%{__install} -p -d -m 0755 %{buildroot}%{tmd_conf_dir}
####%{__install} -p -D -m 0644 %{SOURCE22} %{buildroot}%{tmd_conf_dir}/tmd3_main.conf
####%{__install} -p -D -m 0644 %{SOURCE23} %{buildroot}%{tmd_conf_dir}/tmd3_http.conf
####%{__install} -p -D -m 0644 %{SOURCE24} %{buildroot}%{tmd_conf_dir}/tmd3_ip.conf
####%{__install} -p -D -m 0644 %{SOURCE25} %{buildroot}%{tmd_conf_dir}/tmd3_loc.conf
####%{__install} -p -D -m 0644 %{SOURCE26} %{buildroot}%{tmd_conf_dir}/tmd3_loc_wireless.conf
####
####%{__install} -p -D -m 0644 %{SOURCE65} %{buildroot}%{tmd_conf_dir}/tmd4_main.conf
####%{__install} -p -D -m 0644 %{SOURCE66} %{buildroot}%{tmd_conf_dir}/tmd4_http.conf
####%{__install} -p -D -m 0644 %{SOURCE67} %{buildroot}%{tmd_conf_dir}/tmd4_domain.conf
####%{__install} -p -D -m 0644 %{SOURCE68} %{buildroot}%{tmd_conf_dir}/tmd4_loc.conf
####%{__install} -p -D -m 0644 %{SOURCE69} %{buildroot}%{tmd_conf_dir}/tmd4_loc.lua
####
####%{__install} -p -d -m 0755 %{buildroot}%{tmd_conf_dir}/tmdconf
####%{__install} -p -D -m 0644 %{SOURCE54} %{buildroot}%{tmd_conf_dir}/tmdconf/check.lua
####%{__install} -p -D -m 0644 %{SOURCE55} %{buildroot}%{tmd_conf_dir}/tmdconf/head
####%{__install} -p -D -m 0644 %{SOURCE56} %{buildroot}%{tmd_conf_dir}/tmdconf/foot
####%{__install} -p -D -m 0644 %{SOURCE57} %{buildroot}%{tmd_conf_dir}/tmdconf/tair_white.lua
####
####%{__install} -p -D -m 0644 %{SOURCE58} %{buildroot}%{tmd_conf_dir}/tmdconf/taobao-wait-head.html
####%{__install} -p -D -m 0644 %{SOURCE59} %{buildroot}%{tmd_conf_dir}/tmdconf/taobao-wait-footer.html
####%{__install} -p -D -m 0644 %{SOURCE60} %{buildroot}%{tmd_conf_dir}/tmdconf/tmall-wait-head.html
####%{__install} -p -D -m 0644 %{SOURCE61} %{buildroot}%{tmd_conf_dir}/tmdconf/tmall-wait-footer.html
####%{__install} -p -D -m 0644 %{SOURCE62} %{buildroot}%{tmd_conf_dir}/tmdconf/doc

####%{__install} -p -d -m 0755 %{buildroot}%{tmd_conf_dir}/x5

# TODO support sec #
####cp -r modules/mod_x5/conf/* %{buildroot}%{tmd_conf_dir}/x5
####
##### WAF conf
####cp modules/ngx_http_waf_module/conf/waf_http.conf %{buildroot}%{tengine_confdir}/waf_http.conf
####cp modules/ngx_http_waf_module/conf/waf_loc.conf %{buildroot}%{tengine_confdir}/waf_loc.conf

# TODO support beacon #
# beacon conf: S5, S6, S49
%define beacon_conf_dir %{tengine_home_data}/beacon-%{tengine_version}-%{release}
%{__install} -p -d -m 0755 %{buildroot}%{beacon_conf_dir}
%{__install} -p -D -m 0644 %{SOURCE5}  %{buildroot}%{beacon_conf_dir}/taobao-beacon.cfg
%{__install} -p -D -m 0644 %{SOURCE6}  %{buildroot}%{beacon_conf_dir}/taobao-channel.cfg
%{__install} -p -D -m 0644 %{SOURCE49} %{buildroot}%{beacon_conf_dir}/wireless-beacon.cfg

# lua
%{__install} -p -D -m 0644 %{SOURCE78} %{buildroot}%{tengine_confdir}/init_by_lua_file.lua
%{__install} -p -D -m 0644 %{_sourcedir}/init_worker_by_lua_file.lua %{buildroot}%{tengine_confdir}/init_worker_by_lua_file.lua


# TODO support comm cfg #
##### ufe
####cp -rf %{SOURCE79}  %{buildroot}%{tengine_confdir}/
####
#####hsf
####cp -rf %{SOURCE82} %{buildroot}%{tengine_libdir}/lua_lib
####
####find $RPM_BUILD_ROOT -name '.svn' -type d -print0|xargs -0 rm -rf
####find %{buildroot} -type f -print0|xargs -0 sed -i -e 's/\/home\/admin\/cai/\/opt\/taobao\/tengine/g'
####
##### hotitem
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/hotitem
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/hotitem/lua_lib
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_confdir}/hotitem/lua_lib/resty
####%{__install} -p -D -m 0644 %{SOURCE83}/hotitem.lua %{buildroot}%{tengine_confdir}/hotitem/hotitem.lua
####%{__install} -p -D -m 0644 %{SOURCE83}/hotitem_conf.lua %{buildroot}%{tengine_confdir}/hotitem/hotitem_conf.lua
####%{__install} -p -D -m 0644 %{SOURCE83}/lua_lib/hotitem.lua %{buildroot}%{tengine_confdir}/hotitem/lua_lib/hotitem.lua
####%{__install} -p -D -m 0644 %{SOURCE83}/lua_lib/store.lua %{buildroot}%{tengine_confdir}/hotitem/lua_lib/store.lua
####%{__install} -p -D -m 0644 %{SOURCE83}/lua_lib/resty/http.lua %{buildroot}%{tengine_confdir}/hotitem/lua_lib/resty/http.lua
####%{__install} -p -D -m 0644 %{SOURCE83}/lua_lib/resty/url.lua %{buildroot}%{tengine_confdir}/hotitem/lua_lib/resty/url.lua
####
##### csp
####%{__install} -p -D -m 0644 %{SOURCE84} %{buildroot}%{tengine_home}/conf/csp_conf/csp_http.conf
####%{__install} -p -D -m 0644 %{SOURCE85} %{buildroot}%{tengine_home}/conf/csp_conf/csp_init.lua
####%{__install} -p -D -m 0644 %{SOURCE86} %{buildroot}%{tengine_home}/conf/csp_conf/csp_body.lua
####%{__install} -p -D -m 0644 %{SOURCE87} %{buildroot}%{tengine_home}/conf/csp_conf/csp_header.lua

%{__install} -p -D -m 0755 %{SOURCE88} %{buildroot}%{tengine_sbindir}/nginx2tengine.sh

# TODO support lua resty rdb #
#####resty
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_home}/lib/lua_lib/resty
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/resty/redis.lua %{buildroot}%{tengine_home}/lib/lua_lib/resty/redis.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/resty/http.lua %{buildroot}%{tengine_home}/lib/lua_lib/resty/http.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/resty/http_headers.lua %{buildroot}%{tengine_home}/lib/lua_lib/resty/http_headers.lua
####
#####lua rdb
####%{__install} -p -d -m 0755 %{buildroot}%{tengine_home}/lib/lua_lib/rdb
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-client.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-client.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-init-worker.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-init-worker.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-restful.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-restful.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-init.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-init.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-client-init-worker.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-client-init-worker.lua
####%{__install} -p -D -m 0644 %{_sourcedir}/lua/rdb/rdb-client-init.lua %{buildroot}%{tengine_home}/lib/lua_lib/rdb/rdb-client-init.lua
####
#lua diamond
%{__install} -p -d -m 0755 %{buildroot}%{tengine_home}/lib/lua_lib/diamond
%{__install} -p -D -m 0644 %{_sourcedir}/lua/diamond/diamond-content-getvar.lua %{buildroot}%{tengine_home}/lib/lua_lib/diamond/diamond-content-getvar.lua

%clean
rm -rf %{buildroot}

%pre
if [ $1 == 1 ]; then
    %{_sbindir}/useradd -c "Nginx user" -s /bin/false -r -d %{tengine_home} %{tengine_user} 2>/dev/null || :
fi
sed '/%{name}-rlog/'d /etc/crontab -i || :
sed '/tmd-rlog/'d /etc/crontab -i || :

%post
#if [ $1 == 1 ]; then
#    /sbin/sysctl -w net.unix.max_dgram_qlen=60000
#fi

sed 's/mod_nginx off/mod_nginx on/' /etc/tsar/tsar.conf -i || :

chmod 777 /dev/shm

install_dir=$(rpm -ql tengine-proxy-%{tengine_version}-%{release} | head -n 1)

if [ ! -d /opt/taobao ]; then
    mkdir -p /opt/taobao
fi
rm -f /opt/taobao/tengine
ln -s ${install_dir} /opt/taobao/tengine

cd ${install_dir}/modules-%{tengine_version}-%{release}
find . -name '*.so' -exec rm -rf ../modules/{} \;
cd ${install_dir}/modules
find . -type l -exec rm -f {} \;
find ../modules-%{tengine_version}-%{release} -maxdepth 1 -mindepth 1 -type f -exec ln -s {} \;
# support sec cfg #
####mkdir tmd4
####cd tmd4
####find ../../modules-%{tengine_version}-%{release}/tmd4 -maxdepth 1 -mindepth 1 -type f -exec ln -s {} \;
####cd ..
find . -user root -exec chown -h admin:admin {} \;
####cd ${install_dir}/data/tmd-%{tengine_version}-%{release}
####find . -maxdepth 1 -mindepth 1 -name 'tmd*' -exec rm -rf ../../conf/{} \;
cd ${install_dir}/data/beacon-%{tengine_version}-%{release}
find . -maxdepth 1 -mindepth 1 -name '*.cfg' -exec rm -rf ../../conf/{} \;
cd ${install_dir}/conf
find . -maxdepth 1 -mindepth 1 -type l -exec rm -f {} \;
####find ../data/tmd-%{tengine_version}-%{release} -maxdepth 1 -mindepth 1 -name 'tmd*' -exec ln -s {} \;
####find ../data/tmd-%{tengine_version}-%{release} -maxdepth 1 -mindepth 1 -name 'x5*' -exec ln -s {} \;
find ../data/beacon-%{tengine_version}-%{release} -maxdepth 1 -mindepth 1 -name '*.cfg' -exec ln -s {} \;
find . -user root -exec chown -h admin:admin {} \;

sp=$(echo %{tengine_datadir} | sed -e 's/\//\\\//g')
tp=$(echo ${install_dir} | sed -e 's/\//\\\//g')

####sed -i -e "s/${sp}/${tp}/g" ${install_dir}/bin/dso-tool


%preun
if [ $1 == 0 ]; then
    sed '/%{name}-rlog/'d /etc/crontab -i || :
    sed '/tmd-rlog/'d /etc/crontab -i || :
    rm -f /opt/taobao/tengine
    install_dir=$(rpm -ql tengine-proxy-%{tengine_version}-%{release} | head -n 1)
    find ${install_dir}/modules -type l -exec rm {} \;
    find ${install_dir}/conf -maxdepth 1 -mindepth 1 -type l -exec rm {} \;
fi

%postun

%files
%defattr(755, root, %{tengine_group}, -)
%{tengine_home_data}
%dir %{tengine_sbindir}
%{tengine_sbindir}/nginxctl
%{tengine_sbindir}/nginx-mem.sh
%{tengine_sbindir}/nginx-admin-ctl
####%{tengine_sbindir}/dso-tool
%{tengine_sbindir}/setup_services.sh
%{tengine_sbindir}/nginx2tengine.sh
%{tengine_sbindir}/reqstatus.py

%attr(6755, root, %{tengine_group}) %{tengine_sbindir}/tengine
%if %{with liaoyuan}
%{tengine_sbindir}/liaoyuan.sh
%endif
%dir %{tengine_datadir}
%dir %{tengine_confdir}
%dir %{tengine_confdir}/apps
# TODO support comm cfg #
####%{tengine_confdir}/gray_strategy
####%{tengine_confdir}/gray_conf
%{tengine_confdir}/admin
####%{tengine_confdir}/ufe
####%{tengine_confdir}/csp_conf
%{tengine_moduledir}
%{tengine_modstodir}
%{tengine_libdir}
%{tengine_confdir}/tair_rest
%dir %{tengine_logdir}
%config(noreplace) %{tengine_confdir}/win-utf
%config(noreplace) %{tengine_confdir}/nginx.conf.default
%config(noreplace) %{tengine_confdir}/mime.types.default
%config(noreplace) %{tengine_confdir}/fastcgi.conf
%config(noreplace) %{tengine_confdir}/fastcgi.conf.default
%config(noreplace) %{tengine_confdir}/fastcgi_params
%config(noreplace) %{tengine_confdir}/fastcgi_params.default
%config(noreplace) %{tengine_confdir}/scgi_params
%config(noreplace) %{tengine_confdir}/scgi_params.default
%config(noreplace) %{tengine_confdir}/uwsgi_params
%config(noreplace) %{tengine_confdir}/uwsgi_params.default
%config(noreplace) %{tengine_confdir}/koi-win
%config(noreplace) %{tengine_confdir}/koi-utf
%config(noreplace) %{tengine_confdir}/nginx-proxy.conf
%config(noreplace) %{tengine_confdir}/nginx-admin.conf
%config(noreplace) %{tengine_confdir}/mime.types
%config(noreplace) %{tengine_confdir}/ip.dat
%config(noreplace) %{tengine_confdir}/sm2tr.txt
####%config(noreplace) %{tengine_confdir}/browsers
%config(noreplace) %{tengine_confdir}/set_user_unit.lua
%config(noreplace) %{tengine_confdir}/cell_main.conf
%config(noreplace) %{tengine_confdir}/cell_server.conf
####%config(noreplace) %{tengine_confdir}/module_stubs

%config(noreplace) %{tengine_confdir}/med_http.conf
%config(noreplace) %{tengine_confdir}/med_srv.conf
%config(noreplace) %{tengine_confdir}/dso.conf
%config(noreplace) %{tengine_confdir}/user.conf
%config(noreplace) %{tengine_confdir}/services.conf
%config(noreplace) %{tengine_confdir}/med.js
%config(noreplace) %{tengine_confdir}/detector.conf
%config(noreplace) %{tengine_confdir}/detector_srv.conf

#sec
%config(noreplace) %{tengine_confdir}/sec_http.conf
%config(noreplace) %{tengine_confdir}/sec_loc.conf
%config(noreplace) %{tengine_confdir}/sec_off.conf
%config(noreplace) %{tengine_confdir}/sinfo.conf

%{tengine_confdir}/init_by_lua_file.lua
%{tengine_confdir}/init_worker_by_lua_file.lua

####%{tengine_confdir}/hotitem

%changelog
* Wed Apr 15 2015 weiyue <weiyue@taobao.com> - 2.0.7
- tmd 4.0 added
- beacon updated
- module vipserver bugfixed, no memory leak now
- nginxctl updated, added 'tengine_status' to test if nginx is working, and changed 'status' to test if nginx is online
- keyless ssl & global session cache added
- req_status updated, added directive 'req_status_add_indicator' to append user-defined fields
- tsar 'nginx' turned on
- module set_misc_more added, shared

* Wed Jan 07 2015 weiyue <weiyue@taobao.com> - 2.0.5
- add substisution, tbip module
- update tengine-proxy-ctl

* Thu Jul 24 2014 weiyue <weiyue@taobao.com> - 2.0.3
- support relocate

* Thu Apr 17 2014 weiyue <weiyue@taobao.com> - 1.2.0
- change install diretory to /opt/taobao/tengine (symbolic link)
- remove service
- change default file privilleges
- change beacon and tmd to dso modules
