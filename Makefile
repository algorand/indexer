cmd/indexer/indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/indexer && go build

idb/setup_postgres_sql.go:	idb/setup_postgres.sql
	cd idb && go generate

importer/protocols_json.go:	importer/protocols.json
	cd importer && go generate

mocks:	idb/dummy.go
	cd idb && mockery -name=IndexerDb

test:	mocks
	go test ./...

.deb_tmp/usr/bin/algorand-indexer:	cmd/indexer/indexer
	mkdir -p .deb_tmp/usr/bin
	rm -f .deb_tmp/usr/bin/indexer
	ln $< $@

SYSTEMD_SOURCES=$(wildcard misc/systemd/*)
SYSTEMD_DEST=$(patsubst misc/systemd/%,.deb_tmp/lib/systemd/system/%,${SYSTEMD_SOURCES})

.deb_tmp/lib/systemd/system/%:	misc/systemd/
	mkdir -p .deb_tmp/lib/systemd/system
	ln $< $@

DEB_CONTROL_FILES=control
DEB_CONTROL_SOURCES=$(patsubst %,misc/debian/%,${DEB_CONTROL_FILES})
DEB_CONTROL_DEST=$(patsubst %,.deb_tmp/DEBIAN/%,${DEB_CONTROL_FILES})

# may need to
# sudo apt-get install -y dpkg-dev
ARCH=$(shell dpkg-architecture -q DEB_HOST_ARCH)
VER=$(shell cat .version)

.deb_tmp/DEBIAN/%:	misc/debian/%
	mkdir -p .deb_tmp/DEBIAN
	sed -e "s,@ARCH@,${ARCH}," -e "s,@VER@,${VER}," < $< > $@

.deb_tmp/DEBIAN/copyright:	LICENSE misc/debian_make_copyright.sh
	mkdir -p .deb_tmp/DEBIAN
	bash misc/debian_make_copyright.sh

algorand_indexer.deb:	.deb_tmp/usr/bin/algorand-indexer ${SYSTEMD_DEST} ${DEB_CONTROL_DEST} .deb_tmp/DEBIAN/copyright
#	for i in control; do echo "foo ${i} bar"; if [ ! -f ".deb_tmp/DEBIAN/${i}" ]; then sed -e "s,@ARCH@,${ARCH}," -e "s,@VER@,${VER}," < "misc/debian/${i}" > ".deb_tmp/DEBIAN/${i}"; fi; done)
#	chmod +x .deb_tmp/DEBIAN/{postinst,postrm,preinst,prerm}
	dpkg-deb --build .deb_tmp algorand_indexer.deb

.PHONY:
