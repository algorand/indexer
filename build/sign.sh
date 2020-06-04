#!/usr/bin/env bash

set -exo pipefail

echo
date "+build_indexer begin SIGN stage %Y%m%d_%H%M%S"
echo

WORKDIR=$(pwd)
FULLVERSION=${VERSION:-$("$WORKDIR/scripts/compute_build_number.sh")}
PKG_DIR="$WORKDIR/packages/$FULLVERSION"
SIGNING_KEY_ADDR=dev@algorand.com
EXTENSIONS=(deb tar.bz2)

make_hashes () {
    # Start clean.
    rm -f ./hashes*

    for ext in ${EXTENSIONS[*]}
    do
        HASHFILE="hashes_${FULLVERSION}_${ext}"
        {
            md5sum ./*"$FULLVERSION"*."$ext" ;
            shasum -a 256 ./*"$FULLVERSION"*."$ext" ;
            shasum -a 512 ./*"$FULLVERSION"*."$ext" ;
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
#git archive --prefix="algorand-indexer-$FULLVERSION/" "$BRANCH" | gzip >| "algorand-indexer_source_${FULLVERSION}.tar.gz"
make_sigs
make_hashes
popd

echo
date "+build_indexer end SIGN stage %Y%m%d_%H%M%S"
echo

