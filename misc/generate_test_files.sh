#!/usr/bin/env bash
#
# Dependencies:
#   - psql, on Ubuntu: 'sudo apt install postgresql-client-12'
#   - Start the SDK test environment to setup a DB at localhost:5433
#
# Arguments (modes)
#
# data [query] - print all encoded transactions, useful to quickly find which
#                TXIDs are useful for whatever test is being written.
#                Optionally provide a query.

export PGPASSWORD='harness'

TEMP_FILE='/tmp/copy_tmp'


# Arg1 TXID        txid to download using 'escape' notation, the leading slash will be added back in.
#                  For example: x34325755373246543447434c4f494b374a325546414d56585a33355233365455334c344954575a53464b32364643364d4a414951
# Arg2 FILE_PATH   location to put the binary file
function download_txn() {
  psql -U algorand --password -d dataset2 -p 5433 -h localhost -w -c "\copy (select encode(txnbytes, 'hex') from txn where txid='\\$1') TO '$TEMP_FILE'" > /dev/null
  xxd -p -r $TEMP_FILE > $2
  rm $TEMP_FILE
}

if [[ $1 == "data" ]]; then
  echo "Transaction Report"
  echo "--------------"

  if [[ $2 != "" ]]; then
    psql -qtAX -U algorand --password -d dataset2 -p 5433 -h localhost -w -c "$2" > rows
  else
    # Grab all txids if no query was provided
    psql -qtAX -U algorand --password -d dataset2 -p 5433 -h localhost -w -c 'select txid from txn where typeenum=6' > rows
  fi

  while read p; do
    download_txn "$p" "temp_stxn"
    echo "TXID: $p"
    msgpacktool -d < "temp_stxn"
    echo "--------------"
  done < rows

  # cleanup
  rm temp_stxn
  rm rows

  exit
fi

APAN_OPTIN="x534132584b4142364f534b34565a54484653534d42415248594437374e48534f3533364c453435505936474d4f575843434b3351"
APAN_CLOSE="x41434a415059424d47504a364b535650543651364834564a5544445158524f435a364c5a345032424f364b504532574e354b5551"
APAN_CLEAR="x50515234574843474f485736354555375a464656465942444756524437425756475733535737374c555557424932474453424441"
APAN_UPDATE="x56565237525552354635555337334c434d505951484c53544f435a5042464f515759484d5a4e4a43464943374849455a43455651"
APAN_DELETE="x4b4a4a4842465954454d354c52515137513251444d58574d365732364c4555365452335157514d4356374c545146475356553541"
NON_ASCII_KEY="x4a35535a4e574b5033463759424f59585634564b4d5744424a53424a52584748415a5953343558424e4254434d4a4e494f503441"
APAN_CALL_1="x374454423353494b5a37485541564e3654494f4454514d324d353743523249434657343343424f4d554e4a425736425255564d41"
APAN_CALL_2="x424245355635463433424f4f44444832355552523646533455324f43425a554f5834514d3744415457544a594e584c4d43434141"
APAN_CALL_3="x4a41494b5a594834484f42344e544536444650354f444a564249334542465250545851334f585649504b5645415a354f51524241"
FOREIGN_APP="x4f5350585a5246503458484754595151564537354a5946425132445542505059484346445a414237545a50555353533759504151"

REKEY="x494f49514e4e46515635485349515543564d4f534d374f4d4d3349373642503656414c534f5a524547423453334a56534a445451"

download_txn $APAN_OPTIN ../api/test_resources/app_optin.txn
download_txn $APAN_CLOSE ../api/test_resources/app_close.txn
download_txn $APAN_CLEAR ../api/test_resources/app_clear.txn
download_txn $APAN_UPDATE ../api/test_resources/app_update.txn
download_txn $APAN_DELETE ../api/test_resources/app_delete.txn
download_txn $NON_ASCII_KEY ../api/test_resources/app_nonascii.txn
download_txn $APAN_CALL_1 ../api/test_resources/app_call_1.txn
download_txn $APAN_CALL_2 ../api/test_resources/app_call_2.txn
download_txn $APAN_CALL_3 ../api/test_resources/app_call_3.txn
download_txn $FOREIGN_APP ../api/test_resources/app_foreign.txn
download_txn $REKEY ../api/test_resources/rekey.txn
