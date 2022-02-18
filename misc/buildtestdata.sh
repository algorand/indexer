#!/usr/bin/env bash
# requires go-algorand checked out at $GOALGORAND or "${GOPATH}/src/github.com/algorand/go-algorand"
#
# Builds data to $E2EDATA or "${HOME}/Algorand/e2edata"
#
# If $BUILD_BLOCK_ARCHIVE is not empty, the blocks will be extracted and archived
#
# Requires Python with py-algorand-sdk installed.
#
# usage:
#    #!/usr/bin/env bash
#    rm -rf ve3
#    export GOALGORAND="${GOPATH}/src/github.com/algorand/go-algorand"
#    export E2EDATA="${HOME}/algorand/indexer/e2edata"
#    export BUILD_BLOCK_ARCHIVE="yes please"
#    rm -rf "$E2EDATA"
#    mkdir -p "$E2EDATA"
#    python3 -m venv ve3
#    ve3/bin/pip install py-algorand-sdk
#    . ve3/bin/activate
#    ./misc/buildtestdata.sh

set -x
set -e

if [ -z "${GOALGORAND}" ]; then
    echo "Using default GOALGORAND"
    GOALGORAND="${GOPATH}/src/github.com/algorand/go-algorand"
fi

if [ -z "${E2EDATA}" ]; then
    echo "Using default E2EDATA"
    E2EDATA="${HOME}/Algorand/e2edata"
fi

tests="${INDEXER_BTD_TESTS:-"test/scripts/e2e_subs/{*.py,*.sh}"}"
echo "Configured tests = ${tests}"

# TODO: EXPERIMENTAL
# run faster rounds? 1000 down from 2000
export ALGOSMALLLAMBDAMSEC=1000

rm -rf "${E2EDATA}"
mkdir -p "${E2EDATA}"
(cd "${GOALGORAND}" && \
  TEMPDIR="${E2EDATA}" \
  python3 test/scripts/e2e_client_runner.py --keep-temps "$tests")

(cd "${E2EDATA}" && tar -j -c -f net_done.tar.bz2 --exclude node.log --exclude agreement.cdv net)

#RSTAMP=$(python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() + time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')
RSTAMP=$(TZ=UTC python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() - time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')

echo "COPY AND PASTE THIS TO UPLOAD:"
echo aws s3 cp --acl public-read "${E2EDATA}/net_done.tar.bz2" s3://algorand-testdata/indexer/e2e3/"${RSTAMP}"/net_done.tar.bz2
