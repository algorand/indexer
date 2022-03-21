#!/usr/bin/env bash

set -x
set -e

CUR_DIR=$(pwd)

# We need to have a local directory as temp because
# adding it to the docker container requires that we
# stay within the "root" of the Dockerfile
# meaning we can't traverse paths outside it

# Setting E2EDATA will affect buildtestdata.sh as well
export E2EDATA="${CUR_DIR}/local_e2e"
export LOCALTMP="${CUR_DIR}/local_tmp"
export LOCALBINDIR="${LOCALTMP}/bin"
export LOCALDATADIR="${LOCALTMP}/data"
export BUILD_BLOCK_ARCHIVE="yes please"

echo "GOALGORAND: ${GOALGORAND}"
echo "E2EDATA: ${E2EDATA}"

rm -rf "$E2EDATA"
mkdir -p "$E2EDATA"

rm -rf "$LOCALTMP"
mkdir -p "$LOCALTMP"

rm -rf "$LOCALBINDIR"
mkdir -p "$LOCALBINDIR"

rm -rf "$LOCALDATADIR"
mkdir -p "$LOCALDATADIR"

pushd "$LOCALBINDIR" || exit
curl https://raw.githubusercontent.com/algorand/go-algorand/1e1474216421da27008726c44ebe0a5ba2fb6a08/cmd/updater/update.sh -o update.sh
chmod +x update.sh
./update.sh -i -c nightly -p "${LOCALBINDIR}" -d "${LOCALDATADIR}" -n
popd

export PATH="${LOCALBINDIR}:${PATH}"

rm -rf ve3
python3 -m venv ve3
ve3/bin/pip install py-algorand-sdk
. ve3/bin/activate
./buildtestdata.sh

docker-compose build --build-arg GO_IMAGE=${GO_IMAGE} && docker-compose up --exit-code-from e2e