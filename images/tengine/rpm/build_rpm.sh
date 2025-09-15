
rm -rf ~/rpmbuild
mkdir -p ~/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

cp ../rootfs/source/tengine-* ~/rpmbuild/SOURCES
cp ../rootfs/source/jemalloc-* ~/rpmbuild/SOURCES
cp ../rootfs/source/BabaSSL-* ~/rpmbuild/SOURCES
cp ../rootfs/source/zlib-* ~/rpmbuild/SOURCES
cp ../rootfs/source/luajit* ~/rpmbuild/SOURCES
cp ../rootfs/source/pcre-* ~/rpmbuild/SOURCES

cp tengine.spec ~/rpmbuild/SPECS

rpmbuild -ba ~/rpmbuild/SPECS/tengine.spec
