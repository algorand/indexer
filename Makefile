# This is the default target, build the indexer:
cmd/indexer/indexer:	idb/setup_postgres_sql.go importer/protocols_json.go .PHONY
	cd cmd/indexer && CGO_ENABLED=0 go build

# may need to
# sudo apt-get install -y dpkg-dev
DEB_ARCH?=$(shell dpkg-architecture -q DEB_HOST_ARCH)
VER=$(shell cat .version)

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
	rm -f $@
	ln $< $@

SYSTEMD_FILES=algorand-indexer.service algorand-indexer@.service
SYSTEMD_DEST=$(patsubst %,.deb_tmp/lib/systemd/system/%,${SYSTEMD_FILES})
SYSTEMD_TARDEST=$(patsubst %,.tar_tmp/algorand-indexer_${VER}/%,${SYSTEMD_FILES})

.deb_tmp/lib/systemd/system/%:	misc/systemd/%
	mkdir -p .deb_tmp/lib/systemd/system
	rm -f $@
	ln $< $@

DEB_CONTROL_FILES=control
DEB_CONTROL_SOURCES=$(patsubst %,misc/debian/%,${DEB_CONTROL_FILES})
DEB_CONTROL_DEST=$(patsubst %,.deb_tmp/DEBIAN/%,${DEB_CONTROL_FILES})

.deb_tmp/DEBIAN/%:	misc/debian/%
	mkdir -p .deb_tmp/DEBIAN
	sed -e "s,@ARCH@,${DEB_ARCH}," -e "s,@VER@,${VER}," < $< > $@

.deb_tmp/DEBIAN/copyright:	LICENSE misc/debian_make_copyright.sh
	mkdir -p .deb_tmp/DEBIAN
	bash misc/debian_make_copyright.sh

algorand-indexer.deb:	.deb_tmp/usr/bin/algorand-indexer ${SYSTEMD_DEST} ${DEB_CONTROL_DEST} .deb_tmp/DEBIAN/copyright
#	chmod +x .deb_tmp/DEBIAN/{postinst,postrm,preinst,prerm}
	dpkg-deb --build .deb_tmp algorand-indexer_${VER}_${DEB_ARCH}.deb
	rm -f algorand-indexer.deb
	ln algorand-indexer_${VER}_${DEB_ARCH}.deb algorand-indexer.deb

.tar_tmp/algorand-indexer_${VER}/%:	misc/systemd/%
	mkdir -p .tar_tmp/algorand-indexer_${VER}
	rm -f $@
	ln $< $@

.tar_tmp/algorand-indexer_${VER}/algorand-indexer:	cmd/indexer/indexer
	mkdir -p .tar_tmp/algorand-indexer_${VER}
	rm -f $@
	ln $< $@

.tar_tmp/algorand-indexer_${VER}/%:	%
	mkdir -p .tar_tmp/algorand-indexer_${VER}
	rm -f $@
	ln $< $@

algorand-indexer.tar.bz2:	cmd/indexer/indexer ${SYSTEMD_TARDEST} .tar_tmp/algorand-indexer_${VER}/algorand-indexer .tar_tmp/algorand-indexer_${VER}/LICENSE .tar_tmp/algorand-indexer_${VER}/README.md
	tar -c -j -f algorand-indexer_${VER}.tar.bz2 -C .tar_tmp algorand-indexer_${VER}
	rm -f algorand-indexer.tar.bz2
	ln algorand-indexer_${VER}.tar.bz2 algorand-indexer.tar.bz2

package:	algorand-indexer.tar.bz2 algorand-indexer.deb .PHONY

.PHONY:
