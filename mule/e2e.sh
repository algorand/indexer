#!/usr/bin/env bash
# This is normally run inside the docker container defined by docker/Dockerfile.mule
# and invoked by TravisCI as specified in .travis.yml.

set -ex

ARCH=$(./mule/scripts/archtype.sh)
OS_TYPE=$(./mule/scripts/ostype.sh)
VERSION=$(./mule/scripts/compute_build_number.sh)
DEB="./tmp/node_pkgs/$OS_TYPE/$ARCH/$VERSION/algorand-indexer_${VERSION}_amd64.deb"
DB_NAME=e2e_tests
PORT=5432

dpkg -i "$DEB"

/etc/init.d/postgresql start
sudo -u postgres bash -c "psql -c \"CREATE ROLE $USER WITH SUPERUSER CREATEDB LOGIN ENCRYPTED PASSWORD '$USER';\""
sudo -u postgres bash -c "psql -c \"CREATE DATABASE $DB_NAME;\""

python3 misc/e2etest.py --connection-string "host=localhost port=$PORT dbname=$DB_NAME sslmode=disable user=$USER password=$USER" --indexer-bin /usr/bin/algorand-indexer

