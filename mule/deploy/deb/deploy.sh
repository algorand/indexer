#!/usr/bin/env bash

set -ex

WORKDIR="$1"

if [ -z "$WORKDIR" ]
then
    echo "WORKDIR variable must be defined."
    exit 1
fi

echo
date "+build_release begin DEPLOY stage %Y%m%d_%H%M%S"
echo

OS_TYPE=$(uname | awk '{print tolower($0)}')
ARCH_BIT=$(uname -m)

if [[ "$ARCH_BIT" = "x86_64" ]]; then
    ARCH_TYPE="amd64"
elif [[ "$ARCH_BIT" = "armv6l" ]]; then
    ARCH_TYPE="arm"
elif [[ "$ARCH_BIT" = "armv7l" ]]; then
    ARCH_TYPE="arm"
elif [[ "$ARCH_BIT" = "aarch64" ]]; then
    ARCH_TYPE="arm64"
else
    # Anything else needs to be specifically added...
    echo "unsupported"
    exit 1
fi

#VERSION=${VERSION:-$4}

#BRANCH=${BRANCH:-$(git rev-parse --abbrev-ref HEAD)}
#CHANNEL=${CHANNEL:-$("$WORKDIR/scripts/compute_branch_channel.sh" "$BRANCH")}
CHANNEL=stable
PKG_DIR="$WORKDIR/tmp/node_pkgs/$OS_TYPE/$ARCH_TYPE"
SIGNING_KEY_ADDR=dev@algorand.com

chmod 400 "$HOME/.gnupg"

#if ! $USE_CACHE
#then
#    export ARCH_BIT
#    export ARCH_TYPE
#    export CHANNEL
#    export OS_TYPE
#    export VERSION
#
#    mule -f package-deploy.yaml package-deploy-setup-deb
#fi

apt-get install aptly -y

cat <<EOF>"$HOME/.aptly.conf"
{
  "rootDir": "$HOME/aptly",
  "downloadConcurrency": 4,
  "downloadSpeedLimit": 0,
  "architectures": [],
  "dependencyFollowSuggests": false,
  "dependencyFollowRecommends": false,
  "dependencyFollowAllVariants": false,
  "dependencyFollowSource": false,
  "dependencyVerboseResolve": false,
  "gpgDisableSign": false,
  "gpgDisableVerify": false,
  "gpgProvider": "gpg",
  "downloadSourcePackages": false,
  "skipLegacyPool": true,
  "ppaDistributorID": "ubuntu",
  "ppaCodename": "",
  "skipContentsPublishing": false,
  "FileSystemPublishEndpoints": {},
  "S3PublishEndpoints": {
    "ben-test-2.0.3": {
      "region":"us-east-1",
      "bucket":"ben-test-2.0.3",
      "acl":"public-read",
      "prefix":"deb-indexer"
    }
  },
  "SwiftPublishEndpoints": {}
}
EOF

#DEBS_DIR="$HOME/packages/deb/$CHANNEL"
DEB="$PKG_DIR/algorand-indexer_${VERSION}_${ARCH_TYPE}.deb"

#cp "$PKG_DIR/$DEB" "$DEBS_DIR"

SNAPSHOT="${CHANNEL}-${VERSION}"
aptly repo create -distribution="$CHANNEL" -component=main algorand-indexer
#aptly repo add algorand "$DEBS_DIR"/*.deb
aptly repo add algorand-indexer "$DEB"
aptly snapshot create "$SNAPSHOT" from repo algorand-indexer
aptly publish snapshot -gpg-key="$SIGNING_KEY_ADDR" -origin=Algorand -label=Algorand "$SNAPSHOT" "s3:ben-test-2.0.3:"

echo
date "+build_release end DEPLOY stage %Y%m%d_%H%M%S"
echo

