#!/usr/bin/env bash
# This is normally run inside the docker container defined by docker/Dockerfile.mule
# and invoked by TravisCI as specified in .travis.yml.

set -ex

apt-get install -y gnupg2 curl software-properties-common

# Run this at this stage rather than in Dockerfile.mule to make sure we always have the latest algod. The other packages are more cacheable.
curl https://releases.algorand.com/key.pub | apt-key add -
add-apt-repository "deb https://releases.algorand.com/deb/ stable main"
apt-get update
apt-get install -y algorand

ARCH=$(./mule/scripts/archtype.sh)
OS_TYPE=$(./mule/scripts/ostype.sh)
VERSION=$(./mule/scripts/compute_build_number.sh)
DEB="./tmp/node_pkgs/$OS_TYPE/$ARCH/$VERSION/algorand-indexer_${VERSION}_amd64.deb"
DB_NAME=e2e_tests
PORT=5432
PATH="${PATH}":/usr/bin
export PATH

# TODO: delete following line which is debugging to see what docker env is doing in automated test
which goal
/usr/bin/goal -h

dpkg -i "$DEB"

/etc/init.d/postgresql start
sudo -u postgres bash -c "psql -c \"CREATE ROLE $USER WITH SUPERUSER CREATEDB LOGIN ENCRYPTED PASSWORD '$USER';\""
sudo -u postgres bash -c "psql -c \"CREATE DATABASE $DB_NAME;\""

python3 misc/e2elive.py --connection-string "host=localhost port=$PORT dbname=$DB_NAME sslmode=disable user=$USER password=$USER" --indexer-bin /usr/bin/algorand-indexer
