#!/usr/bin/env bash
# shellcheck disable=2035

set -exo pipefail

echo
date "+build_indexer begin SIGN stage %Y%m%d_%H%M%S"
echo

ARCH=${ARCH:-(./mule/scripts/archtype.sh)}
OS_TYPE=${OS_TYPE:-$(./mule/scripts/ostype.sh)}
VERSION=$(./mule/scripts/compute_build_number.sh)
PKG_DIR="./tmp/node_pkgs/$OS_TYPE/$ARCH/$VERSION"
SIGNING_KEY_ADDR=dev@algorand.com
EXTENSIONS=(deb tar.bz2)

make_hashes () {
    # Start clean.
    rm -f ./hashes*

    for ext in ${EXTENSIONS[*]}
    do
        HASHFILE="hashes_${VERSION}_${ext}"
        {
            md5sum *"$VERSION"*."$ext" ;
            shasum -a 256 *"$VERSION"*."$ext" ;
            shasum -a 512 *"$VERSION"*."$ext" ;
        } >> "$HASHFILE"

        gpg -u "$SIGNING_KEY_ADDR" --detach-sign "$HASHFILE"
        gpg -u "$SIGNING_KEY_ADDR" --clearsign "$HASHFILE"
    done
}

make_sigs () {
    # Start clean.
    rm -f ./*.sig

    for item in ./*
    do
        gpg -u "$SIGNING_KEY_ADDR" --detach-sign "$item"
    done
}

pushd "$PKG_DIR"
#git archive --prefix="algorand-indexer-$VERSION/" "$BRANCH" | gzip >| "algorand-indexer_source_${VERSION}.tar.gz"
make_sigs
make_hashes
popd

echo
date "+build_indexer end SIGN stage %Y%m%d_%H%M%S"
echo

