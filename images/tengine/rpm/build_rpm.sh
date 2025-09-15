
mkdir -p ~/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

cp ../source/tengine-* ~/rpmbuild/SOURCES
cp ../source/jemalloc-* ~/rpmbuild/SOURCES
cp ../source/BabaSSL-* ~/rpmbuild/SOURCES

cp tengine.spec ~/rpmbuild/SPECS

rpmbuild -ba ~/rpmbuild/SPECS/tengine.spec
