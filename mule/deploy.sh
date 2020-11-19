#!/usr/bin/env bash

set -ex

echo
date "+build_indexer begin DEPLOY stage %Y%m%d_%H%M%S"
echo

chmod 400 "$HOME/.gnupg"

if [ -z "$STAGING" ]
then
    echo "[$0] Staging is a required parameter."
    exit 1
fi

if [ -z "$CHANNEL" ]
then
    echo "[$0] Channel is a required parameter."
    exit 1
fi

if [[ ! "$CHANNEL" =~ ^beta$|^stable$ ]]
then
    echo "[$0] Repository values must be either \`beta\` or \`stable\`."
    exit 1
fi

if [ -z "$VERSION" ]
then
    echo "[$0] Version is a required parameter."
    exit 1
fi

if [ -z "$SNAPSHOT" ]
then
    SNAPSHOT="$CHANNEL-$VERSION"
fi

aws cloudfront create-invalidation --distribution-id E14LR4GBB5ZIHD --paths "/*"

aptly mirror update indexer

KEY_PREFIX="indexer/$VERSION"
FILENAME_SUFFIX="${VERSION}_amd64.deb"
INDEXER_KEY="$KEY_PREFIX/algorand-indexer_${FILENAME_SUFFIX}"

PACKAGES_DIR=/root/packages
mkdir -p /root/packages

if aws s3api head-object --bucket "$STAGING" --key "$INDEXER_KEY"
then
    aws s3 cp "s3://$STAGING/$INDEXER_KEY" "$PACKAGES_DIR"
else
    echo "[$0] The package \`$INDEXER_KEY\` failed to download."
    exit 1
fi

if ls -A $PACKAGES_DIR
then
    aptly repo add indexer "$PACKAGES_DIR"/*.deb
    aptly repo show -with-packages indexer
    aptly snapshot create "$SNAPSHOT" from repo indexer
    if ! aptly publish show indexer s3:algorand-releases: &> /dev/null
    then
        aptly publish snapshot -gpg-key=dev@algorand.com -origin=Algorand -label=Algorand "$SNAPSHOT" s3:algorand-releases:
    else
        aptly publish switch indexer s3:algorand-releases: "$SNAPSHOT"
    fi

else
    echo "[$0] The packages directory is empty, so there is nothing to add the \`$CHANNEL\` repo."
    exit 1
fi

echo
date "+build_indexer end DEPLOY stage %Y%m%d_%H%M%S"
echo

