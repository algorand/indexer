#!/bin/bash
# requires go-algorand checked out at $GOALGORAND or "${GOPATH}/src/github.com/algorand/go-algorand"
#
# Builds data to $E2EDATA or "${HOME}/Algorand/e2edata"
#
# Requires Python with py-algorand-sdk installed.
#
# usage:
# python3 -m venv ve3
# ve3/bin/pip install py-algorand-sdk
# bash
# . ve3/bin/activate
# bash misc/buildtestdata.sh

set -x
set -e

if [ -z "${GOALGORAND}" ]; then
    GOALGORAND="${GOPATH}/src/github.com/algorand/go-algorand"
fi

if [ -z "${E2EDATA}" ]; then
    E2EDATA="${HOME}/Algorand/e2edata"
fi

# TODO: EXPERIMENTAL
# run faster rounds? 1000 down from 2000
export ALGOSMALLLAMBDAMSEC=1000

rm -rf "${E2EDATA}"
mkdir -p "${E2EDATA}"
(cd "${GOALGORAND}/test/scripts" && TEMPDIR="${E2EDATA}" python3 e2e_client_runner.py --keep-temps e2e_subs/*.sh)

(cd "${E2EDATA}" && tar -j -c -f net_done.tar.bz2 --exclude node.log --exclude agreement.cdv net)

#RSTAMP=$(python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() + time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')
RSTAMP=$(TZ=UTC python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() - time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')

echo "COPY AND PASTE THIS TO UPLOAD:"
echo aws s3 cp --acl public-read "${E2EDATA}/net_done.tar.bz2" s3://algorand-testdata/indexer/e2e2/${RSTAMP}/net_done.tar.bz2
