#!/usr/bin/env bash

# Test block processing by hooking up indexer to preconfigured block datasets.

set -e

# This script only works when CWD is 'test/migrations'
rootdir=`dirname $0`
pushd $rootdir > /dev/null
pwd

source common.sh
trap cleanup EXIT

start_postgres

###############
## RUN TESTS ##
###############
# Test 1
kill_indexer
start_indexer_with_blocks createdestroy blockdata/create_destroy.tar.bz2
wait_for_ready

#####################
# Application Tests #
#####################
query_and_verify "app create (app-id=203)" createdestroy \
  "select created_at, closed_at, index from app WHERE index = 203" \
  "55||203"
query_and_verify "app create & delete (app-id=82)" createdestroy \
  "select created_at, closed_at, index from app WHERE index = 82" \
  "13|37|82"

###############
# Asset Tests #
###############
query_and_verify "asset create / destroy" createdestroy \
  "select created_at, closed_at, index from asset WHERE index=135" \
  "23|33|135"
query_and_verify "asset create" createdestroy \
  "select created_at, closed_at, index from asset WHERE index=168" \
  "35||168"

###########################
# Application Local Tests #
###########################
query_and_verify "app optin no closeout" createdestroy \
  "select created_at, closed_at, app from account_app WHERE addr=decode('rAMD0F85toNMRuxVEqtxTODehNMcEebqq49p/BZ9rRs=', 'base64') AND app=85" \
  "13||85"
query_and_verify "app multiple optins first saved" createdestroy \
  "select created_at, closed_at, app from account_app WHERE addr=decode('Eze95btTASDFD/t5BDfgA2qvkSZtICa5pq1VSOUU0Y0=', 'base64') AND app=82" \
  "15|35|82"
query_and_verify "app optin/optout/optin should clear closed_at" createdestroy \
  "select created_at, closed_at, app from account_app WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND app=203" \
  "57||203"

#######################
# Asset Holding Tests #
#######################
query_and_verify "asset optin" createdestroy \
  "select created_at, closed_at, assetid from account_asset WHERE addr=decode('MFkWBNGTXkuqhxtNVtRZYFN6jHUWeQQxqEn5cUp1DGs=', 'base64') AND assetid=27" \
  "13||27"
query_and_verify "asset optin / close-out" createdestroy \
  "select created_at, closed_at, assetid from account_asset WHERE addr=decode('E/p3R9m9X0c7eAv9DapnDcuNGC47kU0BxIVdSgHaFbk=', 'base64') AND assetid=36" \
  "16|25|36"
query_and_verify "asset optin / close-out / optin / close-out" createdestroy \
  "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=135" \
  "25|31|135"
query_and_verify "asset optin / close-out / optin" createdestroy \
  "select created_at, closed_at, assetid from account_asset WHERE addr=decode('ZF6AVNLThS9R3lC9jO+c7DQxMGyJvOqrNSYQdZPBQ0Y=', 'base64') AND assetid=168" \
  "37||168"

#################
# Account Tests #
#################
query_and_verify "genesis account with no transactions" createdestroy \
  "select created_at, closed_at, microalgos from account WHERE addr = decode('4L294Wuqgwe0YXi236FDVI5RX3ayj4QL1QIloIyerC4=', 'base64')" \
  "0||5000000000000000"
query_and_verify "account created then never closed" createdestroy \
  "select created_at, closed_at, microalgos from account WHERE addr = decode('HoJZm6Z2n0EvGncuitv2BA7m8Gu/Y9rx22ZtKw1BbjI=', 'base64')" \
  "4||999999885998"
query_and_verify "account create close create" createdestroy \
  "select created_at, closed_at, microalgos from account WHERE addr = decode('KbUa0wk9gB3BgAjQF0J9NqunWaFS+h4cdZdYgGfBes0=', 'base64')" \
  "17||100000"
query_and_verify "account create close create close" createdestroy \
  "select created_at, closed_at, microalgos from account WHERE addr = decode('8rpfPsaRRIyMVAnrhHF+SHpq9za99C1NknhTLGm5Xkw=', 'base64')" \
  "9|15|0"
