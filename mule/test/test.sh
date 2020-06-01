#!/usr/bin/env bash

set -ex

export WORKDIR="$1"

if [ -z "$WORKDIR" ]
then
    echo "WORKDIR must be defined."
    exit 1
fi

OS_TYPE=$("$WORKDIR/scripts/ostype.sh")
export OS_TYPE
ARCH=$("$WORKDIR/scripts/archtype.sh")
export ARCH
FULLVERSION=${VERSION:-$("$WORKDIR/scripts/compute_build_number.sh")}
export FULLVERSION
BRANCH=${BRANCH:-$(git rev-parse --abbrev-ref HEAD)}
export BRANCH
CHANNEL=${CHANNEL:-$("$WORKDIR/scripts/compute_branch_channel.sh" "$BRANCH")}
export CHANNEL
SHA=${SHA:-$(git rev-parse HEAD)}
export SHA

if ! $USE_CACHE
then
    mule -f mule.yaml "package-setup-deb"
fi

"$WORKDIR/mule/test/util/test_package.sh"

