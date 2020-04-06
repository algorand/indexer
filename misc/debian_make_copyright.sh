#!/bin/bash
cat <<EOF> ".deb_tmp/DEBIAN/copyright"
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: Algorand Indexer
Upstream-Contact: Algorand developers <dev@algorand.com>
Source: https://github.com/algorand/indexer

Files: *
Copyright: Algorand developers <dev@algorand.com>
License: AGPL-3+
EOF
sed 's/^$/./g' < LICENSE | sed 's/^/ /g' >> ".deb_tmp/DEBIAN/copyright"
