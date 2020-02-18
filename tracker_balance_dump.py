#!/usr/bin/env python3
#
# Dump algod tracker database to show all current algo and asset balances.
# Prints json per line {"addr":"", "v":{"algo":0, "123":0}, "f":{"123":true}}
#
# usage:
#  python tracker_balance_dump.py -f mainnet-v1.0/ledger.tracker.sqlite
#
# setup requires:
#  pip install msgpack py-algorand-sdk

import json
import logging
import os
import sqlite3
import sys
import urllib.request
import urllib.parse

import msgpack
import algosdk
import psycopg2

from indexer2testload import unmsgpack

logger = logging.getLogger(__name__)

def indexerAccounts(rooturl, blockround=None):
    rootparts = urllib.parse.urlparse(rooturl)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(accountsurl[2], 'v1/accounts')
    gtaddr = None
    accounts = {}
    query = {}
    if blockround is not None:
        query['r'] = blockround
    while True:
        if gtaddr:
            # set query:
            query['gt'] = gtaddr
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        response = urllib.request.urlopen(pageurl)
        if (response.code != 200) or (response.getheader('Content-Type') != 'application/json'):
            raise Exception("bad response to {!r}: {}".format(pageurl, response.reason))
        ob = json.loads(response.read())
        some = False
        for acct in ob.get('accounts',[]):
            some = True
            addr = acct['address']
            microalgos = acct['amountwithoutpendingrewards']
            av = {"algo":microalgos}
            assets = {}
            for assetid, assetholding in acct.get('assets',{}).items():
                assets[str(assetid)] = {"a":assetholding['amount'],"f":assetholding.get('frozen',False)}
            if assets:
                av['asset'] = assets
            rawaddr = algosdk.encoding.decode_address(addr)
            accounts[rawaddr] = av
            gtaddr = addr
        if not some:
            break
    logger.info('loaded %d accounts from %s', len(accounts), rooturl)
    return accounts

class CheckContext:
    def __init__(self, i2db, err):
        self.i2db = i2db
        self.err = err
        self.match = 0
        self.neq = 0

    def check(self, address, niceaddr, microalgos, assets):
        err = self.err
        i2v = self.i2db.pop(address, None)
        if i2v is None:
            self.neq += 1
            err.write('{} not in i2db\n'.format(niceaddr))
        else:
            ok = True
            if i2v['algo'] == microalgos:
                pass # still ok
            else:
                err.write('{} algod v={} i2db v={}\n'.format(niceaddr, microalgos, i2v['algo']))
                # TODO: fetch txn delta from db?
                # pc = pg.cursor()
                # pc.execute("SELECT t.round, t.intra, t.txn FROM txn t JOIN txn_participation p ON t.round = p.round AND t.intra = p.intra WHERE p.addr = %s AND t.round >= %s and t.round <= %s", (address, min(i2db_round, tracker_round), max(i2db_round, tracker_round)))
                # for row in pc:
                #     blockround, intra, txn = row
                #     err.write('\t{}:{}\t{}\n'.format(blockround, intra, json.dumps(txn['txn'])))
                ok = False
            i2assets = i2v.get('asset')
            if i2assets:
                if assets:
                    pass
                else:
                    ok = False
                    err.write('{} i2db has assets but not algod\n'.format(niceaddr))
            elif assets:
                allzero = True
                nonzero = []
                for assetidstr, assetdata in assets.items():
                    if assetdata[b'a'] != 0:
                        allzero = False
                        nonzero.append( (assetidstr, assetdata) )
                if not allzero:
                    ok = False
                    err.write('{} algod has assets but not i2db: {!r}\n'.format(niceaddr, nonzero))
            if ok:
                self.match += 1
            else:
                self.neq += 1

    def summary(self):
        for addr, i2v in self.i2db.items():
            if i2v.get('algo',0) == 0:
                # we keep records on 0 balance accounts but algod doesn't. okay. ignore that.
                continue
            try:
                niceaddr = algosdk.encoding.encode_address(addr)
            except:
                niceaddr = repr(addr)
            self.err.write('{} in indexer but not algod, v={}\n'.format(niceaddr, i2v))
        self.err.write('{} match {} neq\n'.format(self.match, self.neq))

# data/basics/userBalance.go AccountData
# "onl":uint Status
# "algo":uint64 MicroAlgos

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-f', '--dbfile', default=None, help='sqlite3 tracker file from algod')
    ap.add_argument('--limit', default=None, type=int, help='debug limit number of accoutns to dump')
    ap.add_argument('--i2db', default=None, help='psql connect string for indexer 2 db')
    ap.add_argument('--indexer', default=None, help='URL to indexer to fetch from')
    ap.add_argument('--asset', default=None, help='filter on accounts possessing asset id')
    ap.add_argument('-q', '--quiet', default=False, action='store_true')
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    out = sys.stdout
    err = sys.stderr

    i2db = {}
    i2db_checker = None
    i2db_round = None
    i2algosum = 0
    i2a = None
    i2a_checker = None
    
    if args.i2db:
        pg = psycopg2.connect(args.i2db)
        cursor = pg.cursor()
        cursor.execute("SELECT addr, microalgos, rewardsbase FROM account")
        for row in cursor:
            addr, microalgos, rewardsbase = row
            i2algosum += microalgos
            addr = addr.tobytes()
            i2db[addr] = {"algo":microalgos, "ebase":rewardsbase}
        cursor.execute("SELECT addr, assetid, amount, frozen FROM account_asset")
        for row in cursor:
            addr, assetid, amount, frozen = row
            addr = addr.tobytes()
            acct = i2db.get(addr)
            if acct is None:
                logger.error("addr %r in account-asset but not account", addr)
            else:
                assets = acct.get('asset')
                if assets is None:
                    assets = dict()
                    acct['asset'] = assets
                assets[str(assetid)] = {"a":amount,"f":frozen}
        cursor.execute("SELECT v FROM metastate WHERE k = 'state'")
        i2db_round = None
        for row in cursor:
            ob = row[0]
            i2db_round = ob.get('account_round')
        cursor.close()
        i2db_checker = CheckContext(i2db, err)

    db = sqlite3.connect(args.dbfile)
    cursor = db.cursor()
    tracker_round = None
    cursor.execute("SELECT rnd FROM acctrounds WHERE id = 'acctbase'")
    for row in cursor:
        tracker_round = row[0]
    if args.indexer:
        i2a = indexerAccounts(args.indexer, blockround=tracker_round)
        i2a_checker = CheckContext(i2a, err)
    err.write('tracker round {}, i2db round {}\n'.format(tracker_round, i2db_round))
    cursor.execute('''SELECT address, data FROM accountbase''')
    count = 0
    #match = 0
    #neq = 0
    algosum = 0
    for row in cursor:
        address, data = row
        niceaddr = algosdk.encoding.encode_address(address)
        adata = msgpack.loads(data)
        count += 1
        rewardsbase = adata.get(b'ebase', 0)
        microalgos = adata[b'algo']
        values = {"algo": microalgos}
        assets = adata.get(b'asset')
        frozen = {}
        has_asset = False
        algosum += microalgos
        if assets:
            for assetidstr, assetdata in assets.items():
                if isinstance(assetidstr, bytes):
                    assetidstr = assetidstr.decode()
                elif isinstance(assetidstr, str):
                    pass
                else:
                    assetidstr = str(assetidstr)
                if assetidstr == args.asset:
                    has_asset = True
                values[assetidstr] = assetdata[b'a']
                if assetdata.get(b'f'):
                    frozen[assetidstr] = True
        if args.asset and not has_asset:
            continue
        if i2db_checker:
            i2db_checker.check(address, niceaddr, microalgos, assets)
        if i2a_checker:
            i2a_checker.check(address, niceaddr, microalgos, assets)
        ob = {
            "addr": niceaddr,
            "v": values,
            "ebase": rewardsbase,
        }
        if frozen:
            ob['f'] = frozen
        if not args.quiet:
            out.write(json.dumps(ob) + '\n')
        if args.limit and count > args.limit:
            break
    if i2db_checker:
        err.write('i2db microalgo sum {}, algod {}\n'.format(i2algosum, algosum))
        i2db_checker.summary()
        err.write('tracker round {}, i2db round {}\n'.format(tracker_round, i2db_round))
        pg.close()
    if i2a_checker:
        i2a_checker.summary()
    return


if __name__ == '__main__':
    main()

