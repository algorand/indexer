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

def getAccountList(rooturl):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts')
    query = {}
    accounts = set()
    start = time.time()
    nextToken = None
    while True:
        if nextToken is not None:
            query['next'] = nextToken
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        nextToken = getAccountsPage(pageurl, accounts)
        if nextToken is None:
            break
    dt = time.time() - start
    logger.info('account list: %d accounts in %0.2f seconds, %.1f /s', len(accounts), dt, len(accounts)/dt)
    return accounts

def accountRandom(rooturl, accounts, n=1000, minTime=None, maxTime=30):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accounts = list(accounts)
    random.shuffle(accounts)
    count = 0
    start = time.time()
    for addr in accounts:
        accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts', addr)
        pageurl = urllib.parse.urlunparse(accountsurl)
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

def accountRecents(rooturl, accounts, n=1000, minTime=None, maxTime=30, ntxns=100):
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    atxnsurl = list(rootparts)
    accounts = list(accounts)
    random.shuffle(accounts)
    count = 0
    txcount = 0
    start = time.time()
    for addr in accounts:
        atxnsurl[2] = os.path.join(rawurl[2], 'v2', 'transactions')
        query = {'limit':1000, 'address':addr}
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
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

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--indexer', default='http://localhost:8980', help='URL to indexer to fetch from')
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    accounts = getAccountList(args.indexer)
    accountRandom(args.indexer, accounts, n=1000, minTime=1, maxTime=30)
    accountRecents(args.indexer, accounts, n=1000, minTime=1, maxTime=30)
    return 0

if __name__ == '__main__':
    sys.exit(main())
