#!/usr/bin/env bash

set -ex

git clone https://github.com/btoll/releases-page.git /tmp/releases-page
cd /tmp/releases-page
./generate_releases_page.py >| /tmp/index.html
#mule -f package-deploy.yaml package-deploy-releases-page

