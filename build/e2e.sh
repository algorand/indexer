#!/usr/bin/env bash

set -ex

DB_NAME=e2e_tests
PORT=5432
VERSION=$(./scripts/compute_build_number.sh)

dpkg -i "./packages/$VERSION/algorand-indexer_${VERSION}_amd64.deb"

/etc/init.d/postgresql start
sudo -u postgres bash -c "psql -c \"CREATE ROLE $USER WITH SUPERUSER CREATEDB LOGIN ENCRYPTED PASSWORD '$USER';\""
sudo -u postgres bash -c "psql -c \"CREATE DATABASE $DB_NAME;\""

python3 misc/e2etest.py --connection-string "host=localhost port=$PORT dbname=$DB_NAME sslmode=disable user=$USER password=$USER" --indexer-bin /usr/bin/algorand-indexer

