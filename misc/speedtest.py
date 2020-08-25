#!/usr/bin/env python3

import base64
import json
import logging
import os
import random
import sqlite3
import sys
import time
import urllib.error
import urllib.request
import urllib.parse

logger = logging.getLogger(__name__)

_localhost_rooturl = 'http://localhost:8980'

# getAccountsPage(pageurl string, accounts set())
# return nextToken or None
def getAccountsPage(pageurl, accounts):
    try:
        logger.debug('GET %r', pageurl)
        response = urllib.request.urlopen(pageurl)
    except urllib.error.HTTPError as e:
        logger.error('failed to fetch %r', pageurl)
        logger.error('msg: %s', e.file.read())
        raise
    except Exception as e:
        logger.error('%r', pageurl, exc_info=True)
        raise
    if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
        raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
    ob = json.loads(response.read())
    qa = ob.get('accounts',[])
    if not qa:
        return None
    for acct in qa:
        accounts.add(acct['address'])
    return ob.get('next-token')

# /v2/acccounts
def getAccountList(rooturl):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts')
    query = {}
    accounts = set()
    start = time.time()
    nextToken = None
    reporturl = None
    while True:
        if nextToken is not None:
            query['next'] = nextToken
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        if reporturl is None:
            reporturl = pageurl
        nextToken = getAccountsPage(pageurl, accounts)
        if nextToken is None:
            break
    dt = time.time() - start
    logger.info('account list: %d accounts in %0.2f seconds, %.1f /s', len(accounts), dt, len(accounts)/dt)
    return accounts, {'url':reporturl, 'accounts': len(accounts), 'seconds': dt}

# /v2/accounts/AAAAA
# TODO: multithreading, because a single client won't find the limit of the server
def accountRandom(rooturl, accounts, n=1000, minTime=None, maxTime=30):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accounts = list(accounts)
    random.shuffle(accounts)
    count = 0
    start = time.time()
    reporturl = None
    for addr in accounts:
        accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts', addr)
        pageurl = urllib.parse.urlunparse(accountsurl)
        if reporturl is None:
            reporturl = pageurl
        logger.debug('GET %s', pageurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        # don't actually care about content, just check that it parses
        json.loads(response.read())
        count += 1
        dt = time.time() - start
        if (n and (count >= n)) and (not minTime or (dt > minTime)):
            break
        if not n and minTime and (dt > minTime):
            break
        if maxTime and (dt > maxTime):
            break
    logger.info('account random: %d accounts in %0.2f seconds, %.1f /s', count, dt, count/dt)
    return {'url':reporturl, 'accounts': count, 'seconds': dt}

# /v2/transactions?addr=
# equivalent to
# /v2/accounts/AAAA/transactions
def accountRecents(rooturl, accounts, n=1000, minTime=None, maxTime=10, ntxns=1000):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    atxnsurl = list(rootparts)
    accounts = list(accounts)
    random.shuffle(accounts)
    count = 0
    txcount = 0
    start = time.time()
    reporturl = None
    for addr in accounts:
        atxnsurl[2] = os.path.join(rawurl[2], 'v2', 'transactions')
        query = {'limit':ntxns, 'address':addr}
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
        if reporturl is None:
            reporturl = pageurl
        logger.debug('GET %s', pageurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        ob = json.loads(response.read())
        txns = ob.get('transactions', [])
        txcount += len(txns)
        count += 1
        dt = time.time() - start
        if (n and (count >= n)) and (not minTime or (dt > minTime)):
            break
        if not n and minTime and (dt > minTime):
            break
        if maxTime and (dt > maxTime):
            break
    logger.info('account recent txns: %d accounts in %0.2f seconds, %.1f /s; %d txns, %.1f txn/s', count, dt, count/dt, txcount, txcount/dt)
    return {'url':reporturl, 'accounts': count, 'seconds': dt, 'txns': txcount}

# /v2/assets
def getAssets(rooturl):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(rawurl[2], 'v2', 'assets')
    query = {}
    assets = {}
    start = time.time()
    nextToken = None
    reporturl = None
    while True:
        if nextToken is not None:
            query['next'] = nextToken
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        if reporturl is None:
            reporturl = pageurl
        logger.debug('GET %s', pageurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        page = json.loads(response.read())
        for arec in page.get('assets', []):
            assets[arec['index']] = arec['params']
        nextToken = page.get('next-token')
        if nextToken is None:
            break
    dt = time.time() - start
    logger.info('asset list: %d assets in %0.2f seconds, %.1f /s', len(assets), dt, len(assets)/dt)
    return assets, {'url':reporturl, 'assets': len(assets), 'seconds': dt}

# /v2/assets/{asset-id}/transactions
# equivalent to
# /v2/transactions?asset-id=N
#
# To make fast:
# CREATE INDEX CONCURRENTLY IF NOT EXISTS txn_asset ON txn (asset, round, intra);
def assetTxns(rooturl, assets, n=1000, minTime=None, maxTime=10, ntxns=1000):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    atxnsurl = list(rootparts)
    assets = list(assets.keys())
    random.shuffle(assets)
    count = 0
    txcount = 0
    start = time.time()
    reporturl = None
    for assetid in assets:
        atxnsurl[2] = os.path.join(rawurl[2], 'v2', 'assets', str(assetid), 'transactions')
        query = {'limit':ntxns}
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
        if reporturl is None:
            reporturl = pageurl
        logger.debug('GET %s', pageurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        ob = json.loads(response.read())
        txns = ob.get('transactions', [])
        txcount += len(txns)
        count += 1
        dt = time.time() - start
        if (n and (count >= n)) and (not minTime or (dt > minTime)):
            break
        if not n and minTime and (dt > minTime):
            break
        if maxTime and (dt > maxTime):
            break
    logger.info('asset txns: %d assets in %0.2f seconds, %.1f /s; %d txns, %.1f txn/s', count, dt, count/dt, txcount, txcount/dt)
    return {'url':reporturl, 'assets': count, 'seconds': dt, 'txns': txcount}

# /v2/assets/{asset-id}/balances -- maybe add index to account_asset table?
#
# To make fast:
# CREATE INDEX CONCURRENTLY IF NOT EXISTS account_asset_asset ON account_asset (assetid, addr ASC);
def assetBalances(rooturl, assets, n=1000, minTime=None, maxTime=10, ntxns=1000):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    atxnsurl = list(rootparts)
    assets = list(assets.keys())
    random.shuffle(assets)
    count = 0
    balcount = 0
    start = time.time()
    reporturl = None
    for assetid in assets:
        atxnsurl[2] = os.path.join(rawurl[2], 'v2', 'assets', str(assetid), 'balances')
        query = {'limit':ntxns}
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
        if reporturl is None:
            reporturl = pageurl
        logger.debug('GET %s', pageurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or not response.getheader('Content-Type').startswith('application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        ob = json.loads(response.read())
        bals = ob.get('balances', [])
        balcount += len(bals)
        count += 1
        dt = time.time() - start
        if (n and (count >= n)) and (not minTime or (dt > minTime)):
            break
        if not n and minTime and (dt > minTime):
            break
        if maxTime and (dt > maxTime):
            break
    logger.info('asset balances: %d assets in %0.2f seconds, %.1f /s; %d bals, %.1f bal/s', count, dt, count/dt, balcount, balcount/dt)
    return {'url':reporturl, 'assets': count, 'seconds': dt, 'balances': balcount}

# TODO:
# /v2/applications -- this should be fast because it's a single table scan
# /v2/applications/N -- single row lookup, easy
# /v2/assets/{asset-id} -- single row lookup, easy
# /v2/blocks/{round-number} -- single row lookup, easy
# /v2/transactions what search opts?


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--indexer', default='http://localhost:8980', help='URL to indexer to fetch from, default http://localhost:8980')
    ap.add_argument('--accounts', type=bool, default=True)
    ap.add_argument('--no-accounts', dest='accounts', action='store_false')
    ap.add_argument('--assets', type=bool, default=True)
    ap.add_argument('--no-assets', dest='assets', action='store_false')
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    indexerurl = args.indexer
    if not indexerurl.startswith('http://'):
        indexerurl = 'http://' + indexerurl
    report = []

    if args.accounts:
        accounts, rept = getAccountList(indexerurl)
        report.append(rept)
        rept = accountRandom(indexerurl, accounts, n=1000, minTime=1, maxTime=30)
        report.append(rept)
        rept = accountRecents(indexerurl, accounts, n=1000, minTime=1, maxTime=30)
        report.append(rept)
    if args.assets:
        assets, rept = getAssets(indexerurl)
        report.append(rept)
        rept = assetTxns(indexerurl, assets)
        report.append(rept)
        rept = assetBalances(indexerurl, assets)
        report.append(rept)
    json.dump(report, sys.stdout)
    sys.stdout.write('\n')
    return 0

if __name__ == '__main__':
    sys.exit(main())
