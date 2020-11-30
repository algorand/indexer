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

# TODO: EXPERIMENTAL
# run faster rounds? 1000 down from 2000
export ALGOSMALLLAMBDAMSEC=1000

rm -rf "${E2EDATA}"
mkdir -p "${E2EDATA}"
(cd "${GOALGORAND}/test/scripts" && TEMPDIR="${E2EDATA}" python3 e2e_client_runner.py --keep-temps e2e_subs/*.sh)

(cd "${E2EDATA}" && tar -j -c -f net_done.tar.bz2 --exclude node.log --exclude agreement.cdv net)

if [ ! -z "$BUILD_BLOCK_ARCHIVE" ]; then
# do the long slow build with the extra 320 rounds
LASTDATAROUND=$(sqlite3 "${E2EDATA}"/net/Primary/*/ledger.block.sqlite "SELECT max(rnd) FROM blocks")

echo $LASTDATAROUND

goal network start -r "${E2EDATA}"/net

mkdir -p "${E2EDATA}/blocks"
mkdir -p "${E2EDATA}/blocktars"

python3 misc/blockarchiver.py --algod "${E2EDATA}"/net/Primary --blockdir "${E2EDATA}/blocks" --tardir "${E2EDATA}/blocktars" &
BLOCKARCHIVERPID=$!

ACCTROUND=$(sqlite3 "${E2EDATA}"/net/Primary/*/ledger.tracker.sqlite "SELECT rnd FROM acctrounds WHERE id = 'acctbase'")

while [ ${ACCTROUND} -lt ${LASTDATAROUND} ]; do
    sleep 4
    #goal node status -d "${E2EDATA}"/net/Primary|grep 'Last committed block: '
    ACCTROUND=$(sqlite3 "${E2EDATA}"/net/Primary/*/ledger.tracker.sqlite "SELECT rnd FROM acctrounds WHERE id = 'acctbase'")
done

goal network stop -r "${E2EDATA}"/net

kill $BLOCKARCHIVERPID

mkdir -p "${E2EDATA}/algod/tbd-v1/"
sqlite3 "${E2EDATA}"/net/Primary/*/ledger.tracker.sqlite ".backup '${E2EDATA}/algod/tbd-v1/ledger.tracker.sqlite'"
cp -p "${E2EDATA}/net/Primary/genesis.json" "${E2EDATA}/algod/genesis.json"

python3 misc/blockarchiver.py --just-tar-blocks --blockdir "${E2EDATA}/blocks" --tardir "${E2EDATA}/blocktars"

(cd "${E2EDATA}" && tar jcf e2edata.tar.bz2 blocktars algod)
ls -l "${E2EDATA}/e2edata.tar.bz2"

fi
# end long slow build

#RSTAMP=$(python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() + time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')
RSTAMP=$(TZ=UTC python -c 'import time; print("{:08x}".format(0xffffffff - int(time.time() - time.mktime((2020,1,1,0,0,0,-1,-1,-1)))))')

echo "COPY AND PASTE THIS TO UPLOAD:"
if [ -f "${E2EDATA}/e2edata.tar.bz2" ]; then
    echo aws s3 cp --acl public-read "${E2EDATA}/e2edata.tar.bz2" s3://algorand-testdata/indexer/e2e1/${RSTAMP}/e2edata.tar.bz2
fi
echo aws s3 cp --acl public-read "${E2EDATA}/net_done.tar.bz2" s3://algorand-testdata/indexer/e2e2/${RSTAMP}/net_done.tar.bz2
