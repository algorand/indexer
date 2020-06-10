#!/usr/bin/env bash

set -ex

ARCH=$(./mule/scripts/archtype.sh)
OS_TYPE=$(./mule/scripts/ostype.sh)
VERSION=$(./mule/scripts/compute_build_number.sh)
WORKSPACE=$(pwd)

git clone https://github.com/btoll/releases-page.git /tmp/releases-page
cd /tmp/releases-page
./generate_releases_page.py >| "$WORKSPACE/tmp/node_pkgs/$OS_TYPE/$ARCH/$VERSION/releases.html"

