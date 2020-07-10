#!/usr/bin/env python3
#
# Compare accounting of algod and indexer
#
# setup requires:
#  pip install "msgpack >=1" py-algorand-sdk

import base64
import json
import logging
import os
import sqlite3
import sys
import time
import urllib.error
import urllib.request
import urllib.parse

#import msgpack
import algosdk

from util import maybedecode, mloads, unmsgpack

logger = logging.getLogger(__name__)

reward_addr = base64.b64decode("/v////////////////////////////////////////8=")
fee_addr = base64.b64decode("x/zNsljw1BicK/i21o7ml1CGQrCtAB8x/LkYw1S6hZo=")

def getGenesisVars(genesispath):
    with open(genesispath) as fin:
        genesis = json.load(fin)
        rwd = genesis.get('rwd')
        if rwd is not None:
            global reward_addr
            reward_addr = algosdk.encoding.decode_address(rwd)
        fees = genesis.get('fees')
        if fees is not None:
            global fee_addr
            fee_addr = algosdk.encoding.decode_address(fees)

def encode_addr(addr):
    if len(addr) == 44:
        addr = base64.b64decode(addr)
    if len(addr) == 32:
        return algosdk.encoding.encode_address(addr)
    return 'unknown addr? {!r}'.format(addr)

def indexerAccountsFromAddrs(rooturl, blockround=None, addrlist=None):
    '''return {raw account: {"algo":N, "asset":{}}, ...}'''
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    accounts = {}
    query = {}
    if blockround is not None:
        query['round'] = blockround
    for addr in addrlist:
        accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts', addr)
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        logger.debug('GET %s', pageurl)
        getAccountsPage(pageurl, accounts)
    logger.info('loaded %d accounts from %s ?round=%d', len(accounts), rooturl, blockround)
    return accounts

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
    logger.debug('ob keys %s', ob.keys())
    some = False
    batchcount = 0
    gtaddr = None
    qa = ob.get('accounts',[])
    acct = ob.get('account')
    if acct is not None:
        qa.append(acct)
    for acct in qa:
        batchcount += 1
        some = True
        addr = acct['address']
        rawaddr = algosdk.encoding.decode_address(addr)
        accounts[rawaddr] = acct#av
        gtaddr = addr
    logger.debug('got %d accounts', batchcount)
    return gtaddr, some

def indexerAccounts(rooturl, blockround=None, addrlist=None):
    '''return {raw account: {"algo":N, "asset":{}}, ...}'''
    if addrlist:
        return indexerAccountsFromAddrs(rooturl, blockround, addrlist)
    rootparts = urllib.parse.urlparse(rooturl)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(accountsurl[2], 'v2', 'accounts')
    gtaddr = None
    start = time.time()
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
        gtaddr, some = getAccountsPage(pageurl, accounts)
        if not some:
            break
    dt = time.time() - start
    logger.info('loaded %d accounts from %s ?round=%d in %.2f seconds', len(accounts), rooturl, blockround, dt)
    return accounts

# generator yielding txns objects
def indexerAccountTxns(rooturl, addr, minround=None, maxround=None):
    niceaddr = algosdk.encoding.encode_address(addr)
    rootparts = urllib.parse.urlparse(rooturl)
    atxnsurl = list(rootparts)
    atxnsurl[2] = os.path.join(atxnsurl[2], 'v2', 'transactions')
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

def assetEquality(indexer, algod):
    #ta = dict(algod)
    errs = []
    #ibyaid = {r['asset-id']:r for r in indexer}
    abyaid = {r['asset-id']:r for r in algod}
    for assetrec in indexer:
        assetid = assetrec['asset-id']
        iv = assetrec.get('amount', 0)
        arec = abyaid.pop(assetid, None)
        if arec is None:
            if iv == 0:
                pass # ok
            else:
                errs.append('asset={!r} indexer had {!r} but algod None'.format(assetid, rec))
        else:
            av = arec.get('amount', 0)
            if av != iv:
                errs.append('asset={!r} indexer {!r} != algod {!r}'.format(assetid, rec, arec))
    for assetid, arec in abyaid.items():
        av = arec.get('amount', 0)
        if av != 0:
            errs.append('asset={!r} indexer had None but algod {!r}'.format(assetid, arec))
    if not errs:
        return None
    return ', '.join(errs)

def deepeq(a, b):
    if a is None:
        if not bool(b):
            return True
    if b is None:
        if not bool(a):
            return True
    if isinstance(a, dict):
        if not isinstance(b, dict):
            return False
        for k,av in a.items():
            if not deepeq(av, b.get(k)):
                return False
        return True
    if isinstance(a, list):
        if not isinstance(b, list):
            return False
        if len(a) != len(b):
            return False
        for va,vb in zip(a,b):
            if not deepeq(va,vb):
                return False
        return True
    return a == b


# CheckContext error collector
class ccerr:
    def __init__(self, errout):
        self.err = errout
        self.errors = []
        self.ok = True
    def __call__(self, msg):
        if not msg:
            return
        self.err.write(msg)
        self.errors.append(msg)
        self.ok = False

class CheckContext:
    def __init__(self, accounts, err):
        # accounts from indexer
        self.accounts = accounts
        self.err = err
        self.match = 0
        self.neq = 0
        # [(addr, "err text"), ...]
        self.mismatches = []

    def check(self, address, niceaddr, microalgos, assets, appparams, applocal):
        # check data from sql or algod vs indexer
        i2v = self.accounts.pop(address, None)
        if i2v is None:
            self.neq += 1
            self.err.write('{} not in indexer\n'.format(niceaddr))
        else:
            xe = ccerr(self.err)
            indexerAlgos = i2v['amount-without-pending-rewards']
            if indexerAlgos == microalgos:
                pass # still ok
            else:
                emsg = 'algod v={} i2 v={}'.format(microalgos, indexerAlgos)
                if address == reward_addr:
                    emsg += ' Rewards account'
                elif address == fee_addr:
                    emsg += ' Fee account'
                xe(emsg)
            i2assets = i2v.get('assets')
            if i2assets:
                if assets:
                    emsg = assetEquality(i2assets, assets)
                    if emsg:
                        xe('{} {}\n'.format(niceaddr, emsg))
                else:
                    allzero = True
                    nonzero = []
                    for assetrec in i2assets:
                        if assetrec.get('amount', 0) != 0:
                            nonzero.append(assetrec)
                            allzero = False
                    if not allzero:
                        ex('{} indexer has assets but not algod: {!r}\n'.format(niceaddr, nonzero))
            elif assets:
                allzero = True
                nonzero = []
                for assetrec in assets.items():
                    if assetrec.get('amount',0) != 0:
                        allzero = False
                        nonzero.append(assetrec)
                if not allzero:
                    emsg = '{} algod has assets but not indexer: {!r}\n'.format(niceaddr, nonzero)
                    xe(emsg)
            i2apar = i2v.get('created-apps')
            if appparams:
                if i2apar:
                    if not deepeq(i2apar, appparams):
                        xe('{} indexer and algod disagree on created app indexer={!r} algod={!r}\n'.format(niceaddr, i2apar, appparams))
                else:
                    xe('{} algod has apar but not indexer: {!r}\n'.format(niceaddr, appparams))
            elif i2apar:
                xe('{} indexer has apar but not algod: {!r}\n'.format(niceaddr, i2apar))
            # TODO: applocal
            if xe.ok:
                self.match += 1
            else:
                self.err.write('{} indexer {!r}\n'.format(niceaddr, sorted(i2v.keys())))
                self.neq += 1
                self.mismatches.append( (address, '\n'.join(xe.errors)) )

    def summary(self):
        for addr, i2v in self.accounts.items():
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



def tvToApiTv(tv):
    tt = tv[b'tt']
    if tt == 1: # bytes
        return {'type':tt, 'bytes':tv[b'tb']}
    return {'type':tt, 'uint':tv[b'ui']}

# convert from msgpack TealKeyValue to api TealKeyValueStore
def tkvToTkvs(tkv):
    if not tkv:
        return None
    return [{'key':k,'value':tvToApiTv(v)} for k,v in tkv.items()]

# msgpack to api
def schemaDecode(sch):
    if sch is None:
        return None
    return {
        'num-uint':sch.get(b'nui'),
        'num-byte-slice':sch.get(b'nbs'),
    }

def check_from_sqlite(args):
    genesispath = args.genesis or os.path.join(os.path.dirname(os.path.dirname(args.dbfile)), 'genesis.json')
    getGenesisVars(genesispath)
    db = sqlite3.connect(args.dbfile)
    cursor = db.cursor()
    tracker_round = None
    cursor.execute("SELECT rnd FROM acctrounds WHERE id = 'acctbase'")
    err = sys.stderr
    for row in cursor:
        tracker_round = row[0]
    if args.indexer:
        i2a = indexerAccounts(args.indexer, blockround=tracker_round, addrlist=(args.accounts and args.accounts.split(',')))
        i2a_checker = CheckContext(i2a, err)
    else:
        i2a_checker = None
    cursor.execute('''SELECT address, data FROM accountbase''')
    count = 0
    #match = 0
    #neq = 0
    algosum = 0
    acctset = args.accounts and args.accounts.split(',')
    for row in cursor:
        address, data = row
        niceaddr = algosdk.encoding.encode_address(address)
        if acctset and niceaddr not in acctset:
            continue
        adata = mloads(data)
        logger.debug('%s algod %r', niceaddr, sorted(adata.keys()))
        count += 1
        rewardsbase = adata.get(b'ebase', 0)
        microalgos = adata[b'algo']
        algosum += microalgos
        rawassetholdingmap = adata.get(b'asset')
        if rawassetholdingmap:
            assets = []
            for assetid, rawassetholding in rawassetholdingmap.items():
                apiholding = {
                    'asset-id':assetid,
                    'amount':rawassetholding.get(b'a', 0),
                    'is-frozen':rawassetholding.get(b'b', False),
                }
                assets.append(apiholding)
        else:
            assets = None
        rawAssetParams = adata.get(b'apar')
        if rawAssetParams:
            assetp = []
            for assetid, ap in rawAssetParams.items():
                appparams.append({
                    'clawback':ap.get(b'c'),
                    'creator':niceaddr,
                    'decimals':ap.get(b'dc',0),
                    'default-frozen':ap.get(b'df',False),
                    'freeze':ap.get(b'f'),
                    'manager':ap.get(b'm'),
                    'metadata-hash':ap.get(b'am'),
                    'name':ap.get(b'an'),
                    'reserve':ap.get(b'r'),
                    'total':ap.get(b't', 0),
                    'unit-name':ap.get(b'un'),
                    'url':ap.get(b'au'),
                })
        else:
            assetp = None
        rawappparams = adata.get(b'appp',{})
        appparams = []
        for appid, ap in rawappparams.items():
            appparams.append({
                'id':appid,
                'params':{
                    'approval-program':ap.get(b'approv'),
                    'clear-state-program':ap.get(b'clearp'),
                    'creator':niceaddr,
                    'global-state':tkvToTkvs(ap.get(b'gs')),
                    'global-state-schema':schemaDecode(ap.get(b'gsch')),
                    'local-state-schema':schemaDecode(ap.get(b'lsch')),
                },
            })
        rawapplocal = adata.get(b'appl',{})
        applocal = []
        for appid, als in rawapplocal.items():
            applocal.append({
                'id':appid,
                'state':{
                    'schema':schemaDecode(ap.get(b'hsch')),
                    'key-value':tkvToTkvs(ap.get(b'tkv')),
                },
            })
        if i2a_checker:
            i2a_checker.check(address, niceaddr, microalgos, assets, appparams, applocal)
        if args.limit and count > args.limit:
            break
    return tracker_round, i2a_checker

def token_addr_from_algod(algorand_data):
    addr = open(os.path.join(algorand_data, 'algod.net'), 'rt').read().strip()
    if not addr.startswith('http'):
        addr = 'http://' + addr
    token = open(os.path.join(algorand_data, 'algod.token'), 'rt').read().strip()
    return token, addr

def check_from_algod(args):
    getGenesisVars(os.path.join(args.algod, 'genesis.json'))
    token, addr = token_addr_from_algod(args.algod)
    algod = algosdk.algod.AlgodClient(token, addr)
    status = algod.status()
    lastround = status['lastRound']
    if args.indexer:
        i2a = indexerAccounts(args.indexer, blockround=lastround)
        i2a_checker = CheckContext(i2a, sys.stderr)
    else:
        logger.warn('no indexer, nothing to do')
        return None, None
    rawad = []
    for address in i2a.keys():
        niceaddr = algosdk.encoding.encode_address(address)
        rawad.append(algod.account_info(niceaddr))
    logger.debug('ad %r', rawad[:5])
    #raise Exception("TODO")
    for ad in rawad:
        #logger.debug('ad %r', ad)
        niceaddr = ad['address']
        round = ad['round']
        if round != lastround:
            # re fetch indexer account at round algod got it at
            na = indexerAccounts(args.indexer, blockround=round, addrlist=[niceaddr])
            i2a.update(na)
        microalgos = ad['amountwithoutpendingrewards']
        #values = {"algo": microalgos}
        xa = {}
        for assetidstr, assetdata in ad.get('assets',{}).items():
            #values[assetidstr] = assetdata.get('amount', 0)
            xa[int(assetidstr)] = {'a':assetdata.get('amount', 0), 'f': assetdata.get('frozen', False)}
        address = algosdk.encoding.decode_address(niceaddr)
        i2a_checker.check(address, niceaddr, microalgos, xa)
    return lastround, i2a_checker

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-f', '--dbfile', default=None, help='sqlite3 tracker file from algod')
    ap.add_argument('-d', '--algod', default=None, help='algorand data dir')
    ap.add_argument('--limit', default=None, type=int, help='debug limit number of accoutns to dump')
    ap.add_argument('--indexer', default=None, help='URL to indexer to fetch from')
    ap.add_argument('--asset', default=None, help='filter on accounts possessing asset id')
    ap.add_argument('--mismatches', default=10, type=int, help='max number of mismatches to show details on (0 for no limit)')
    ap.add_argument('-q', '--quiet', default=False, action='store_true')
    ap.add_argument('--verbose', default=False, action='store_true')
    ap.add_argument('--genesis')
    ap.add_argument('--accounts', help='comma separated list of accounts to test')
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    out = sys.stdout
    err = sys.stderr

    i2algosum = 0
    i2a = None
    i2a_checker = None

    if args.dbfile:
        tracker_round, i2a_checker = check_from_sqlite(args)
    elif args.algod:
        tracker_round, i2a_checker = check_from_algod(args)
    else:
        raise Exception("need to check from algod sqlite or running algod")
    if i2a_checker:
        i2a_checker.summary()
        logger.info('txns...')
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
                intra = txn.pop('intra-round-offset')
                parts = ['{}:{}'.format(cround, intra), 's={}'.format(sender)]
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
                    parts.append('as={}'.format(asnd))
                if receiver:
                    parts.append('r={}'.format(receiver))
                if closeto:
                    parts.append('c={}'.format(closeto))
                if arcv:
                    parts.append('ar={}'.format(arcv))
                if aclose:
                    parts.append('ac={}'.format(aclose))
                # everything else
                parts.append(json.dumps(txn))
                err.write(' '.join(parts) + '\n')
            if tmore:
                err.write('... and {} more\n'.format(tmore))
    return


if __name__ == '__main__':
    main()
