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

import algosdk
from algosdk.v2client.algod import AlgodClient

from util import maybedecode, mloads, unmsgpack

logger = logging.getLogger(__name__)

reward_addr = base64.b64decode("/v////////////////////////////////////////8=")
fee_addr = base64.b64decode("x/zNsljw1BicK/i21o7ml1CGQrCtAB8x/LkYw1S6hZo=")
reward_niceaddr = algosdk.encoding.encode_address(reward_addr)
fee_niceaddr = algosdk.encoding.encode_address(fee_addr)

def getGenesisVars(genesispath):
    with open(genesispath) as fin:
        genesis = json.load(fin)
        rwd = genesis.get('rwd')
        if rwd is not None:
            global reward_addr
            global reward_niceaddr
            logger.debug('reward addr %s', rwd)
            reward_addr = algosdk.encoding.decode_address(rwd)
            reward_niceaddr = rwd
        fees = genesis.get('fees')
        if fees is not None:
            global fee_addr
            global fee_niceaddr
            logger.debug('fee addr %s', fees)
            fee_addr = algosdk.encoding.decode_address(fees)
            fee_niceaddr = fees

def encode_addr(addr):
    if len(addr) == 44:
        addr = base64.b64decode(addr)
    if len(addr) == 32:
        return algosdk.encoding.encode_address(addr)
    return 'unknown addr? {!r}'.format(addr)

def encode_addr_or_none(addr):
    if addr is None or not addr:
        return None
    return encode_addr(addr)

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
        if addr in (reward_niceaddr, fee_niceaddr):
            continue
        accountsurl[2] = os.path.join(rawurl[2], 'v2', 'accounts', addr)
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        logger.debug('GET %s', pageurl)
        getAccountsPage(pageurl, accounts)
    logger.debug('loaded %d accounts from %s ?round=%d', len(accounts), rooturl, blockround)
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
        accounts[rawaddr] = acct
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
    logger.debug('loaded %d accounts from %s ?round=%s in %.2f seconds', len(accounts), rooturl, blockround, dt)
    return accounts

# generator yielding txns objects
def indexerAccountTxns(rooturl, addr, minround=None, maxround=None, limit=None):
    niceaddr = algosdk.encoding.encode_address(addr)
    rootparts = urllib.parse.urlparse(rooturl)
    atxnsurl = list(rootparts)
    atxnsurl[2] = os.path.join(atxnsurl[2], 'v2', 'transactions')
    query = {'limit':1000, 'address':niceaddr}
    if minround is not None:
        query['min-round'] = minround
    if maxround is not None:
        query['max-round'] = maxround
    if limit is not None:
        query['limit'] = limit
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
    logger.debug('asset eq? i=%r a=%r', indexer, algod)
    for assetrec in indexer:
        assetid = assetrec['asset-id']
        iv = assetrec.get('amount', 0)
        arec = abyaid.pop(assetid, None)
        if arec is None:
            if iv == 0:
                pass # ok
            else:
                errs.append('asset={!r} indexer had {!r} but algod None'.format(assetid, assetrec))
        else:
            av = arec.get('amount', 0)
            if av != iv:
                errs.append('asset={!r} indexer {!r} != algod {!r}'.format(assetid, assetrec, arec))
    for assetid, arec in abyaid.items():
        av = arec.get('amount', 0)
        if av != 0:
            errs.append('asset={!r} indexer had None but algod {!r}'.format(assetid, arec))
    if not errs:
        return None
    return ', '.join(errs)

def deepeq(a, b, path=None, msg=None):
    if a is None:
        if not bool(b):
            return True
    if b is None:
        if not bool(a):
            return True
    if isinstance(a, dict):
        if not isinstance(b, dict):
            return False
        if len(a) != len(b):
            ak = set(a.keys())
            bk = set(b.keys())
            both = ak.intersection(bk)
            onlya = ak.difference(both)
            onlyb = bk.difference(both)
            mp = []
            if onlya:
                mp.append('only in a {!r}'.format(sorted(onlya)))
            if onlyb:
                mp.append('only in b {!r}'.format(sorted(onlyb)))
            msg.append(', '.join(mp))
            return False
        for k,av in a.items():
            if path is not None and msg is not None:
                subpath = path + (k,)
            else:
                subpath = None
            if not deepeq(av, b.get(k), subpath, msg):
                if msg is not None:
                    msg.append('at {!r}'.format(subpath))
                return False
        return True
    if isinstance(a, list):
        if not isinstance(b, list):
            return False
        if len(a) != len(b):
            return False
        for va,vb in zip(a,b):
            if not deepeq(va,vb,path,msg):
                if msg is not None:
                    msg.append('{!r} != {!r}'.format(va,vb))
                return False
        return True
    return a == b

def remove_indexer_only_fields(x):
    x.pop('deleted', None)
    x.pop('created-at-round', None)
    x.pop('deleted-at-round', None)
    x.pop('destroyed-at-round', None)
    x.pop('optin-at-round', None)
    x.pop('opted-in-at-round', None)
    x.pop('opted-out-at-round', None)
    x.pop('closeout-at-round', None)
    x.pop('closed-out-at-round', None)
    x.pop('closed-at-round', None)

def _dac(x):
    out = dict(x)
    remove_indexer_only_fields(out)
    return out

def dictifyAssetConfig(acfg):
    return {x['index']:_dac(x) for x in acfg}

def _dap(x):
    out = dict(x)
    remove_indexer_only_fields(out)
    out['params'] = dict(out['params'])
    gs = out['params'].get('global-state')
    if gs:
        out['params']['global-state'] = {z['key']:z['value'] for z in gs}
    return out

def dictifyAppParams(ap):
    # make a list of app params comparable by deepeq
    return {x['id']:_dap(x) for x in ap}

def dictifyAppLocal(al):
    out = {}
    for ent in al:
        ent = dict(ent)
        appid = ent.pop('id')
        kv = ent.get('key-value')
        if kv:
            ent['key-value'] = {x['key']:x['value'] for x in kv}
        remove_indexer_only_fields(ent)
        out[appid] = ent
    return out

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

    def check(self, address, niceaddr, microalgos, assets, acfg, appparams, applocal):
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
                        xe('{} indexer has assets but not algod: {!r}\n'.format(niceaddr, nonzero))
            elif assets:
                allzero = True
                nonzero = []
                for assetrec in assets:
                    if assetrec.get('amount',0) != 0:
                        allzero = False
                        nonzero.append(assetrec)
                if not allzero:
                    emsg = '{} algod has assets but not indexer: {!r}\n'.format(niceaddr, nonzero)
                    xe(emsg)
            i2acfg = i2v.get('created-assets')
            # filter out deleted entries that indexer is showing to us
            if i2acfg:
                i2acfg = list(filter(lambda x: x['params']['total'] is not 0, i2acfg))
            if acfg:
                if i2acfg:
                    indexerAcfg = dictifyAssetConfig(i2acfg)
                    algodAcfg = dictifyAssetConfig(acfg)
                    eqerr = []
                    if not deepeq(indexerAcfg, algodAcfg, (), eqerr):
                        xe('{} indexer and algod disagree on acfg, {}\nindexer={}\nalgod={}\n'.format(niceaddr, eqerr, json_pp(i2acfg), json_pp(acfg)))
                else:
                    xe('{} algod has acfg but not indexer: {!r}\n'.format(niceaddr, acfg))
            elif i2acfg:
                xe('{} indexer has acfg but not algod: {!r}\n'.format(niceaddr, i2acfg))
            i2apar = i2v.get('created-apps')
            # filter out deleted entries that indexer is showing to us
            if i2apar:
                i2apar = list(filter(lambda x: x['params']['approval-program'] is not None and x['params']['clear-state-program'] is not None, i2apar))

            if appparams:
                if i2apar:
                    i2appById = dictifyAppParams(i2apar)
                    algodAppById = dictifyAppParams(appparams)
                    eqerr=[]
                    if not deepeq(i2appById, algodAppById,(),eqerr):
                        xe('{} indexer and algod disagree on created app, {}\nindexer={}\nalgod={}\n'.format(niceaddr, eqerr, json_pp(i2appById), json_pp(algodAppById)))
                else:
                    xe('{} algod has apar but not indexer: {!r}\n'.format(niceaddr, appparams))
            elif i2apar:
                xe('{} indexer has apar but not algod: {!r}\n'.format(niceaddr, i2apar))
            i2applocal = i2v.get('apps-local-state')
            # filter out deleted entries that indexer is showing to us
            if i2applocal:
                i2applocal = list(filter(lambda x: x['schema']['num-byte-slice'] is not 0 or x['schema']['num-uint'] is not 0, i2applocal))
            if applocal:
                if i2applocal:
                    eqerr = []
                    ald = dictifyAppLocal(applocal)
                    ild = dictifyAppLocal(i2applocal)
                    if not deepeq(ald, ild, (), eqerr):
                        xe('{} indexer and algod disagree on app local, {}\nindexer={}\nalgod={}\n'.format(niceaddr, eqerr, json_pp(i2applocal), json_pp(applocal)))
                else:
                    xe('{} algod has app local but not indexer: {!r}'.format(niceaddr, applocal))
            elif i2applocal:
                xe('{} indexer has app local but not algod: {!r}'.format(niceaddr, i2applocal))

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


def jsonable(x, display=False):
    if x is None:
        return x
    if isinstance(x, dict):
        return {str(jsonable(k, display)):jsonable(v, display) for k,v in x.items()}
    if isinstance(x, list):
        return [jsonable(e, display) for e in x]
    if isinstance(x, bytes):
        if display:
            return 'b:' + base64.b64encode(x).decode()
        else:
            return base64.b64encode(x).decode()
    return x


def json_pp(x):
    return json.dumps(jsonable(x, True), indent=2, sort_keys=True)

def b64_or_none(x):
    if x is None or not x:
        return None
    return base64.b64encode(x).decode()

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
        'num-uint':sch.get(b'nui',0),
        'num-byte-slice':sch.get(b'nbs',0),
    }

def dmaybe(d, k, v):
    if v is None:
        return
    d[k] = v

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
                params = {
                    'creator':niceaddr,
                    'decimals':ap.get(b'dc',0),
                    'default-frozen':ap.get(b'df',False),
                    'total':ap.get(b't', 0),
                }
                dmaybe(params, 'clawback', encode_addr_or_none(ap.get(b'c')))
                dmaybe(params, 'freeze', encode_addr_or_none(ap.get(b'f')))
                dmaybe(params, 'manager', encode_addr_or_none(ap.get(b'm')))
                dmaybe(params, 'metadata-hash', encode_addr_or_none(ap.get(b'am')))
                dmaybe(params, 'name', maybedecode(ap.get(b'an')))
                dmaybe(params, 'reserve', encode_addr_or_none(ap.get(b'r')))
                dmaybe(params, 'unit-name', maybedecode(ap.get(b'un')))
                dmaybe(params, 'url', maybedecode(ap.get(b'url')))
                assetp.append({
                    'index':assetid,
                    'params':params
                })
        else:
            assetp = None
        rawappparams = adata.get(b'appp',{})
        appparams = []
        for appid, ap in rawappparams.items():
            appparams.append({
                'id':appid,
                'params':{
                    'approval-program':maybeb64(ap.get(b'approv')),
                    'clear-state-program':maybeb64(ap.get(b'clearp')),
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
            # TODO: assets created by this account
            i2a_checker.check(address, niceaddr, microalgos, assets, assetp, appparams, applocal)
        if args.limit and count > args.limit:
            break
    return i2a_checker

def maybeb64(x):
    if x:
        return base64.b64encode(x).decode()
    return x

def token_addr_from_args(args):
    if args.algod:
        addr = open(os.path.join(args.algod, 'algod.net'), 'rt').read().strip()
        token = open(os.path.join(args.algod, 'algod.token'), 'rt').read().strip()
    else:
        addr = args.algod_net
        token = args.algod_token
    if not addr.startswith('http'):
        addr = 'http://' + addr
    return token, addr

def check_from_algod(args):
    gpath = args.genesis or os.path.join(args.algod, 'genesis.json')
    getGenesisVars(gpath)
    token, addr = token_addr_from_args(args)
    algod = AlgodClient(token, addr)
    status = algod.status()
    #logger.debug('status %r', status)
    iloadstart = time.time()
    if args.indexer:
        i2a = indexerAccounts(args.indexer, addrlist=(args.accounts and args.accounts.split(',')))
        i2a_checker = CheckContext(i2a, sys.stderr)
    else:
        logger.warn('no indexer, nothing to do')
        return None, None
    iloadtime = time.time() - iloadstart
    logger.info('loaded %d accounts from indexer in %.1f seconds, %.1f a/s', len(i2a), iloadtime, len(i2a)/iloadtime)
    aloadstart = time.time()
    rawad = []
    for address in i2a.keys():
        niceaddr = algosdk.encoding.encode_address(address)
        rawad.append(algod.account_info(niceaddr))
    aloadtime = time.time() - aloadstart
    logger.info('loaded %d accounts from algod in %.1f seconds, %.1f a/s', len(rawad), aloadtime, len(rawad)/aloadtime)
    lastlog = time.time()
    count = 0
    for ad in rawad:
        niceaddr = ad['address']
        # fetch indexer account at round algod got it at
        na = indexerAccounts(args.indexer, blockround=ad['round'], addrlist=[niceaddr])
        i2a.update(na)
        microalgos = ad['amount-without-pending-rewards']
        address = algosdk.encoding.decode_address(niceaddr)
        i2a_checker.check(address, niceaddr, microalgos, ad.get('assets'), ad.get('created-assets'), ad.get('created-apps'), ad.get('apps-local-state'))
        count += 1
        now = time.time()
        if (now - lastlog) > 5:
            logger.info('%d accounts checked, %d mismatches', count, len(i2a_checker.mismatches))
            lastlog = now
    return i2a_checker

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-f', '--dbfile', default=None, help='sqlite3 tracker file from algod')
    ap.add_argument('-d', '--algod', default=None, help='algorand data dir')
    ap.add_argument('--algod-net', default=None, help='algod host:port')
    ap.add_argument('--algod-token', default=None, help='algod token')
    ap.add_argument('--genesis', default=None, help='path to genesis.json')
    ap.add_argument('--limit', default=None, type=int, help='debug limit number of accoutns to dump')
    ap.add_argument('--indexer', default=None, help='URL to indexer to fetch from')
    ap.add_argument('--asset', default=None, help='filter on accounts possessing asset id')
    ap.add_argument('--mismatches', default=10, type=int, help='max number of mismatches to show details on (0 for no limit)')
    ap.add_argument('-q', '--quiet', default=False, action='store_true')
    ap.add_argument('--verbose', default=False, action='store_true')
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
        i2a_checker = check_from_sqlite(args)
    elif args.algod or (args.algod_net and args.algod_token):
        i2a_checker = check_from_algod(args)
    else:
        raise Exception("need to check from algod sqlite or running algod")
    retval = 0
    if i2a_checker:
        i2a_checker.summary()
        logger.info('txns...')
        mismatches = i2a_checker.mismatches
        if args.mismatches and len(mismatches) > args.mismatches:
            mismatches = mismatches[:args.mismatches]
        for addr, msg in mismatches:
            niceaddr = algosdk.encoding.encode_address(addr)
            if addr in (reward_addr, fee_addr):
                logger.debug('skip %s', niceaddr)
                # we know accounting for the special addrs is different
                continue
            retval = 1
            xaddr = base64.b16encode(addr).decode().lower()
            err.write('\n{} \'\\x{}\'\n\t{}\n'.format(niceaddr, xaddr, msg))
            tcount = 0
            tmore = 0
            for txn in indexerAccountTxns(args.indexer, addr, limit=30):
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
    return retval


if __name__ == '__main__':
    sys.exit(main())
