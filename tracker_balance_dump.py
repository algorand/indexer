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

import base64
import json
import logging
import os
import sqlite3
import sys
import urllib.error
import urllib.request
import urllib.parse

import msgpack
import algosdk
import psycopg2

from indexer2testload import unmsgpack

reward_addr = base64.b64decode("/v////////////////////////////////////////8=")
fee_addr = base64.b64decode("x/zNsljw1BicK/i21o7ml1CGQrCtAB8x/LkYw1S6hZo=")

def encode_addr(addr):
    if len(addr) == 44:
        addr = base64.b64decode(addr)
    if len(addr) == 32:
        return algosdk.encoding.encode_address(addr)
    return 'unknown addr? {!r}'.format(addr)

logger = logging.getLogger(__name__)

def indexerAccounts(rooturl, blockround=None):
    rootparts = urllib.parse.urlparse(rooturl)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(accountsurl[2], 'accounts')
    gtaddr = None
    accounts = {}
    query = {'limit':500}
    if blockround is not None:
        query['round'] = blockround
    while True:
        if gtaddr:
            # set query:
            query['next'] = gtaddr
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        try:
            logger.debug('GET %s', pageurl)
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
        some = False
        batchcount = 0
        for acct in ob.get('accounts',[]):
            batchcount += 1
            some = True
            addr = acct['address']
            microalgos = acct['amount-without-pending-rewards']
            av = {"algo":microalgos}
            assets = {}
            for assetrec in acct.get('assets', []):
                assetid = assetrec['asset-id']
                assetamount = assetrec.get('amount', 0)
                assetfrozen = assetrec.get('is-frozen', False)
                assets[str(assetid)] = {"a":assetamount,"f":assetfrozen}
            if assets:
                av['asset'] = assets
            rawaddr = algosdk.encoding.decode_address(addr)
            accounts[rawaddr] = av
            gtaddr = addr
        logger.debug('got %d accounts', batchcount)
        if not some:
            break
    logger.info('loaded %d accounts from %s ?round=%d', len(accounts), rooturl, blockround)
    return accounts

# generator yielding txns objects
def indexerAccountTxns(rooturl, addr, minround=None, maxround=None):
    niceaddr = algosdk.encoding.encode_address(addr)
    rootparts = urllib.parse.urlparse(rooturl)
    atxnsurl = list(rootparts)
    atxnsurl[2] = os.path.join(atxnsurl[2], 'transactions')
    query = {'limit':1000, 'address':niceaddr}
    if minround is not None:
        query['min-round'] = minround
    if maxround is not None:
        query['max-round'] = maxround
    while True:
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
        try:
            logger.debug('GET %s', pageurl)
            response = urllib.request.urlopen(pageurl)
        except urllib.error.HTTPError as e:
            logger.error('failed to fetch %r', pageurl)
            logger.error('msg: %s', e.file.read())
            raise
        except Exception as e:
            logger.error('failed to fetch %r', pageurl)
            logger.error('err %r (%s)', e, type(e))
            raise
        if (response.code != 200) or (not response.getheader('Content-Type').startswith('application/json')):
            raise Exception("bad response to {!r}: {} {}, {!r}".format(pageurl, response.code, response.reason, response.getheader('Content-Type')))
        ob = json.loads(response.read())
        txns = ob.get('transactions')
        if not txns:
            return
        for txn in txns:
            yield txn
        if len(txns) < 1000:
            return
        query['rh'] = txns[-1]['r'] - 1

class CheckContext:
    def __init__(self, i2db, err):
        self.i2db = i2db
        self.err = err
        self.match = 0
        self.neq = 0
        # [(addr, "err text"), ...]
        self.mismatches = []

    def check(self, address, niceaddr, microalgos, assets):
        err = self.err
        i2v = self.i2db.pop(address, None)
        errors = []
        if i2v is None:
            self.neq += 1
            err.write('{} not in i2db\n'.format(niceaddr))
        else:
            ok = True
            if i2v['algo'] == microalgos:
                pass # still ok
            else:
                ok = False
                emsg = 'algod v={} i2 v={}'.format(microalgos, i2v['algo'])
                if address == reward_addr:
                    emsg += ' Rewards account'
                elif address == fee_addr:
                    emsg += ' Fee account'
                err.write('{} {}\n'.format(niceaddr, emsg))
                errors.append(emsg)
                # TODO: fetch txn delta from db?
                # pc = pg.cursor()
                # pc.execute("SELECT t.round, t.intra, t.txn FROM txn t JOIN txn_participation p ON t.round = p.round AND t.intra = p.intra WHERE p.addr = %s AND t.round >= %s and t.round <= %s", (address, min(i2db_round, tracker_round), max(i2db_round, tracker_round)))
                # for row in pc:
                #     blockround, intra, txn = row
                #     err.write('\t{}:{}\t{}\n'.format(blockround, intra, json.dumps(txn['txn'])))
            i2assets = i2v.get('asset')
            if i2assets:
                if assets:
                    pass
                else:
                    ok = False
                    emsg = '{} i2db has assets but not algod\n'.format(niceaddr)
                    err.write(emsg)
                    errors.append(emsg)
            elif assets:
                allzero = True
                nonzero = []
                for assetidstr, assetdata in assets.items():
                    if assetdata.get(b'a',0) != 0:
                        allzero = False
                        nonzero.append( (assetidstr, assetdata) )
                if not allzero:
                    ok = False
                    emsg = '{} algod has assets but not i2db: {!r}\n'.format(niceaddr, nonzero)
                    err.write(emsg)
                    errors.append(emsg)
            if ok:
                self.match += 1
            else:
                self.neq += 1
                self.mismatches.append( (address, '\n'.join(errors)) )

    def summary(self):
        for addr, i2v in self.i2db.items():
            if i2v.get('algo',0) == 0:
                # we keep records on 0 balance accounts but algod doesn't. okay. ignore that.
                continue
            try:
                niceaddr = algosdk.encoding.encode_address(addr)
            except:
                niceaddr = repr(addr)
            emsg = 'in indexer but not algod, v={}'.format(i2v)
            self.mismatches.append( (addr, emsg) )
            self.err.write('{} {}\n'.format(niceaddr, emsg))
            self.neq +=1
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
    ap.add_argument('--dump', default=False, action='store_true')
    ap.add_argument('--mismatches', default=10, type=int, help='max number of mismatches to show details on (0 for no limit)')
    ap.add_argument('-q', '--quiet', default=False, action='store_true')
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
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
                values[assetidstr] = assetdata.get(b'a', 0)
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
        if args.dump:
            # print every record in the tracker sqlite db
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
        err.write('txns...\n')
        mismatches = i2a_checker.mismatches
        if args.mismatches and len(mismatches) > args.mismatches:
            mismatches = mismatches[:args.mismatches]
        for addr, msg in mismatches:
            if addr in (reward_addr, fee_addr):
                # we know accounting for the special addrs is different
                continue
            niceaddr = algosdk.encoding.encode_address(addr)
            xaddr = base64.b16encode(addr).decode().lower()
            err.write('\n{} \'\\x{}\'\n\t{}\n'.format(niceaddr, xaddr, msg))
            tcount = 0
            tmore = 0
            for txn in indexerAccountTxns(args.indexer, addr, minround=tracker_round):
                if tcount > 10:
                    tmore += 1
                    continue
                tcount += 1
                sender = txn.pop('sender')
                cround = txn.pop('confirmed-round')
                intra = '?' # TODO update when model adds this
                parts = ['{}:{}'.format(cround, intra), 's={}'.format(encode_addr(sender))]
                pay = txn.get('payment-transaction', None)
                if pay:
                    receiver = pay.pop('receiver', None)
                    closeto = pay.pop('close-remainder-to', None)
                else:
                    receiver = None
                    closeto = None
                axfer = txn.get('asset-transfer-transaction', None)
                if axfer is not None:
                    asnd = axfer.pop('sender', None)
                    arcv = axfer.pop('receiver', None)
                    aclose = axfer.pop('close-to', None)
                else:
                    asnd = None
                    arcv = None
                    aclose = None
                if asnd:
                    parts.append('as={}'.format(encode_addr(asnd)))
                if receiver:
                    parts.append('r={}'.format(encode_addr(receiver)))
                if closeto:
                    parts.append('c={}'.format(encode_addr(closeto)))
                if arcv:
                    parts.append('ar={}'.format(encode_addr(arcv)))
                if aclose:
                    parts.append('ac={}'.format(encode_addr(aclose)))
                # everything else
                parts.append(json.dumps(txn))
                err.write(' '.join(parts) + '\n')
            if tmore:
                err.write('... and {} more\n'.format(tmore))
    return


if __name__ == '__main__':
    main()

