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
import queue
import signal
from ssl import SSLContext
import struct
import sys
import threading
import time
import urllib.error
import urllib.request
import urllib.parse

import algosdk
from algosdk.v2client.algod import AlgodClient

from e2e_common.util import maybedecode, mloads, unmsgpack

logger = logging.getLogger(__name__)

reward_addr = base64.b64decode("/v////////////////////////////////////////8=")
fee_addr = base64.b64decode("x/zNsljw1BicK/i21o7ml1CGQrCtAB8x/LkYw1S6hZo=")
reward_niceaddr = algosdk.encoding.encode_address(reward_addr)
fee_niceaddr = algosdk.encoding.encode_address(fee_addr)

ssl_no_validate = None

graceful_stop = False


def do_graceful_stop(signum, frame):
    global graceful_stop
    if graceful_stop:
        sys.stderr.write("second signal, quitting\n")
        sys.exit(1)
    sys.stderr.write("graceful stop...\n")
    graceful_stop = True


signal.signal(signal.SIGTERM, do_graceful_stop)
signal.signal(signal.SIGINT, do_graceful_stop)


def process_genesis(genesis):
    rwd = genesis.get("rwd")
    if rwd is not None:
        global reward_addr
        global reward_niceaddr
        logger.debug("reward addr %s", rwd)
        reward_addr = algosdk.encoding.decode_address(rwd)
        reward_niceaddr = rwd
    fees = genesis.get("fees")
    if fees is not None:
        global fee_addr
        global fee_niceaddr
        logger.debug("fee addr %s", fees)
        fee_addr = algosdk.encoding.decode_address(fees)
        fee_niceaddr = fees


def getGenesisVars(genesispath):
    with open(genesispath) as fin:
        genesis = json.load(fin)
        process_genesis(genesis)


def encode_addr(addr):
    if len(addr) == 44:
        addr = base64.b64decode(addr)
    if len(addr) == 32:
        return algosdk.encoding.encode_address(addr)
    return "unknown addr? {!r}".format(addr)


def encode_addr_or_none(addr):
    if addr is None or not addr:
        return None
    return encode_addr(addr)


def indexerAccountsFromAddrs(rooturl, blockround=None, addrlist=None, headers=None):
    """generator yielding account records"""
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    query = {}
    if blockround is not None:
        query["round"] = blockround
    for addr in addrlist:
        if addr in (reward_niceaddr, fee_niceaddr):
            continue
        accountsurl[2] = os.path.join(rawurl[2], "v2", "accounts", addr)
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        logger.debug("GET %s", pageurl)
        yield from getAccountsPage(pageurl, headers)


def indexerAccountFromAddr(rooturl, niceaddr, blockround=None, headers=None):
    """return one account record"""
    rootparts = urllib.parse.urlparse(rooturl)
    rawurl = list(rootparts)
    accountsurl = list(rootparts)
    query = {}
    if blockround is not None:
        query["round"] = blockround
    accountsurl[2] = os.path.join(rawurl[2], "v2", "accounts", niceaddr)
    if query:
        accountsurl[4] = urllib.parse.urlencode(query)
    pageurl = urllib.parse.urlunparse(accountsurl)
    logger.debug("GET %s", pageurl)
    acct = None
    for na in getAccountsPage(pageurl, headers):
        if acct is None:
            acct = na
        else:
            raise Exception(
                "indexerAccountFromAddr should only yield one, got a second"
            )
    return acct


def getAccountsPage(pageurl, headers=None, retries=5):
    if headers is None:
        headers = {}
    while True:
        try:
            logger.debug("GET %r", pageurl)
            response = urllib.request.urlopen(
                urllib.request.Request(pageurl, headers=headers),
                context=ssl_no_validate,
            )
            if (response.code != 200) or not response.getheader(
                "Content-Type"
            ).startswith("application/json"):
                if retries <= 0:
                    raise Exception(
                        "bad response to {!r}: {}".format(pageurl, response.reason)
                    )
            else:
                break
        except urllib.error.HTTPError as e:
            logger.error("failed to fetch %r", pageurl)
            logger.error("msg: %s", e.file.read())
            if retries <= 0:
                raise
        except Exception as e:
            logger.error("%r", pageurl, exc_info=True)
            if retries <= 0:
                raise
        retries -= 1
        time.sleep(1)
    ob = json.loads(response.read())
    logger.debug("ob keys %s", ob.keys())
    some = False
    batchcount = 0
    qa = ob.get("accounts", [])
    acct = ob.get("account")
    if acct is not None:
        qa.append(acct)
    for acct in qa:
        batchcount += 1
        yield acct
        some = True
    logger.debug("got %d accounts", batchcount)


def indexerAccounts(rooturl, blockround=None, addrlist=None, headers=None, gtaddr=None):
    """return {raw account: {"algo":N, "asset":{}}, ...}"""
    if addrlist:
        yield from indexerAccountsFromAddrs(rooturl, blockround, addrlist, headers)
        return
    rootparts = urllib.parse.urlparse(rooturl)
    accountsurl = list(rootparts)
    accountsurl[2] = os.path.join(accountsurl[2], "v2", "accounts")
    start = time.time()
    query = {"limit": 500}
    if blockround is not None:
        query["round"] = blockround
    count = 0
    while True:
        if gtaddr:
            # set query:
            query["next"] = gtaddr
        if query:
            accountsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(accountsurl)
        some = False
        gtaddr = None
        for acct in getAccountsPage(pageurl, headers):
            count += 1
            some = True
            yield acct
            addr = acct["address"]
            gtaddr = addr
        if not some:
            break
    dt = time.time() - start
    logger.debug(
        "loaded %d accounts from %s ?round=%s in %.2f seconds",
        count,
        rooturl,
        blockround,
        dt,
    )


# generator yielding txns objects
def indexerAccountTxns(
    rooturl, addr, minround=None, maxround=None, limit=None, token=None
):
    niceaddr = algosdk.encoding.encode_address(addr)
    rootparts = urllib.parse.urlparse(rooturl)
    atxnsurl = list(rootparts)
    atxnsurl[2] = os.path.join(atxnsurl[2], "v2", "transactions")
    query = {"limit": 1000, "address": niceaddr}
    if token is not None:
        headers = {"X-Indexer-API-Token": token}
    else:
        headers = {}
    if minround is not None:
        query["min-round"] = minround
    if maxround is not None:
        query["max-round"] = maxround
    if limit is not None:
        query["limit"] = limit
    while True:
        atxnsurl[4] = urllib.parse.urlencode(query)
        pageurl = urllib.parse.urlunparse(atxnsurl)
        try:
            logger.debug("GET %s", pageurl)
            req = urllib.request.Request(pageurl, headers=headers)
            response = urllib.request.urlopen(pageurl, context=ssl_no_validate)
        except urllib.error.HTTPError as e:
            logger.error("failed to fetch %r", pageurl)
            logger.error("msg: %s", e.file.read())
            raise
        except Exception as e:
            logger.error("failed to fetch %r", pageurl)
            logger.error("err %r (%s)", e, type(e))
            raise
        if (response.code != 200) or (
            not response.getheader("Content-Type").startswith("application/json")
        ):
            raise Exception(
                "bad response to {!r}: {} {}, {!r}".format(
                    pageurl,
                    response.code,
                    response.reason,
                    response.getheader("Content-Type"),
                )
            )
        ob = json.loads(response.read())
        txns = ob.get("transactions")
        if not txns:
            return
        for txn in txns:
            yield txn
        if len(txns) < 1000:
            return
        query["rh"] = txns[-1]["r"] - 1


def assetEquality(indexer, algod):
    # ta = dict(algod)
    errs = []
    # ibyaid = {r['asset-id']:r for r in indexer}
    abyaid = {r["asset-id"]: r for r in algod}
    logger.debug("asset eq? i=%r a=%r", indexer, algod)
    for assetrec in indexer:
        assetid = assetrec["asset-id"]
        iv = assetrec.get("amount", 0)
        arec = abyaid.pop(assetid, None)
        if arec is None:
            if iv == 0:
                pass  # ok
            else:
                errs.append(
                    "asset={!r} indexer had {!r} but algod None".format(
                        assetid, assetrec
                    )
                )
        else:
            av = arec.get("amount", 0)
            if av != iv:
                errs.append(
                    "asset={!r} indexer {!r} != algod {!r}".format(
                        assetid, assetrec, arec
                    )
                )
    for assetid, arec in abyaid.items():
        av = arec.get("amount", 0)
        if av != 0:
            errs.append(
                "asset={!r} indexer had None but algod {!r}".format(assetid, arec)
            )
    if not errs:
        return None
    return ", ".join(errs)


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
                mp.append("only in a {!r}".format(sorted(onlya)))
            if onlyb:
                mp.append("only in b {!r}".format(sorted(onlyb)))
            msg.append(", ".join(mp))
            return False
        for k, av in a.items():
            if path is not None and msg is not None:
                subpath = path + (k,)
            else:
                subpath = None
            if not deepeq(av, b.get(k), subpath, msg):
                if msg is not None:
                    msg.append("at {!r}".format(subpath))
                return False
        return True
    if isinstance(a, list):
        if not isinstance(b, list):
            return False
        if len(a) != len(b):
            return False
        for va, vb in zip(a, b):
            if not deepeq(va, vb, path, msg):
                if msg is not None:
                    msg.append("{!r} != {!r}".format(va, vb))
                return False
        return True
    return a == b


def remove_indexer_only_fields(x):
    x.pop("deleted", None)
    x.pop("created-at-round", None)
    x.pop("deleted-at-round", None)
    x.pop("destroyed-at-round", None)
    x.pop("optin-at-round", None)
    x.pop("opted-in-at-round", None)
    x.pop("opted-out-at-round", None)
    x.pop("closeout-at-round", None)
    x.pop("closed-out-at-round", None)
    x.pop("closed-at-round", None)


def _dac(x):
    out = dict(x)
    remove_indexer_only_fields(out)
    return out


def dictifyAssetConfig(acfg):
    return {x["index"]: _dac(x) for x in acfg}


def _dap(x):
    out = dict(x)
    remove_indexer_only_fields(out)
    out["params"] = dict(out["params"])
    gs = out["params"].get("global-state")
    if gs:
        out["params"]["global-state"] = {z["key"]: z["value"] for z in gs}
    return out


def dictifyAppParams(ap):
    # make a list of app params comparable by deepeq
    return {x["id"]: _dap(x) for x in ap}


def dictifyAppLocal(al):
    out = {}
    for ent in al:
        ent = dict(ent)
        appid = ent.pop("id")
        kv = ent.get("key-value")
        if kv:
            ent["key-value"] = {x["key"]: x["value"] for x in kv}
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
    def __init__(self, err):
        self.err = err
        self.match = 0
        self.neq = 0
        # [(addr, "err text"), ...]
        self.mismatches = []
        self.lock = threading.Lock()

    def check(self, indexer_account, algod_account):
        niceaddr = algod_account["address"]
        address = algosdk.encoding.decode_address(niceaddr)
        xe = ccerr(self.err)

        microalgos = algod_account["amount-without-pending-rewards"]
        indexerAlgos = indexer_account["amount-without-pending-rewards"]
        if indexerAlgos == microalgos:
            pass  # still ok
        else:
            emsg = "algod v={} i2 v={} ".format(microalgos, indexerAlgos)
            if address == reward_addr:
                emsg += " Rewards account"
            elif address == fee_addr:
                emsg += " Fee account"
            xe(emsg)
        assets = algod_account.get("assets")
        i2assets = indexer_account.get("assets")
        if i2assets:
            if assets:
                emsg = assetEquality(i2assets, assets)
                if emsg:
                    xe("{} {}\n".format(niceaddr, emsg))
            else:
                allzero = True
                nonzero = []
                for assetrec in i2assets:
                    if assetrec.get("amount", 0) != 0:
                        nonzero.append(assetrec)
                        allzero = False
                if not allzero:
                    xe(
                        "{} indexer has assets but not algod: {!r}\n".format(
                            niceaddr, nonzero
                        )
                    )
        elif assets:
            allzero = True
            nonzero = []
            for assetrec in assets:
                if assetrec.get("amount", 0) != 0:
                    allzero = False
                    nonzero.append(assetrec)
            if not allzero:
                emsg = "{} algod has assets but not indexer: {!r}\n".format(
                    niceaddr, nonzero
                )
                xe(emsg)
        acfg = algod_account.get("created-assets")
        i2acfg = indexer_account.get("created-assets")
        # filter out deleted entries that indexer is showing to us
        if i2acfg:
            i2acfg = list(filter(lambda x: x["params"]["total"] != 0, i2acfg))
        if acfg:
            if i2acfg:
                indexerAcfg = dictifyAssetConfig(i2acfg)
                algodAcfg = dictifyAssetConfig(acfg)
                eqerr = []
                if not deepeq(indexerAcfg, algodAcfg, (), eqerr):
                    xe(
                        "{} indexer and algod disagree on acfg, {}\nindexer={}\nalgod={}\n".format(
                            niceaddr, eqerr, json_pp(i2acfg), json_pp(acfg)
                        )
                    )
            else:
                xe("{} algod has acfg but not indexer: {!r}\n".format(niceaddr, acfg))
        elif i2acfg:
            xe("{} indexer has acfg but not algod: {!r}\n".format(niceaddr, i2acfg))
        appparams = algod_account.get("created-apps")
        i2apar = indexer_account.get("created-apps")
        # filter out deleted entries that indexer is showing to us
        if i2apar:
            i2apar = list(
                filter(
                    lambda x: x["params"]["approval-program"] is not None
                    and x["params"]["clear-state-program"] is not None,
                    i2apar,
                )
            )

        if appparams:
            if i2apar:
                i2appById = dictifyAppParams(i2apar)
                algodAppById = dictifyAppParams(appparams)
                eqerr = []
                if not deepeq(i2appById, algodAppById, (), eqerr):
                    xe(
                        "{} indexer and algod disagree on created app, {}\nindexer={}\nalgod={}\n".format(
                            niceaddr, eqerr, json_pp(i2appById), json_pp(algodAppById)
                        )
                    )
            else:
                xe(
                    "{} algod has apar but not indexer: {!r}\n".format(
                        niceaddr, appparams
                    )
                )
        elif i2apar:
            xe("{} indexer has apar but not algod: {!r}\n".format(niceaddr, i2apar))
        applocal = algod_account.get("apps-local-state")
        i2applocal = indexer_account.get("apps-local-state")
        # filter out deleted entries that indexer is showing to us
        if i2applocal:
            i2applocal = list(
                filter(
                    lambda x: x["schema"]["num-byte-slice"] != 0
                    or x["schema"]["num-uint"] != 0,
                    i2applocal,
                )
            )
        if applocal:
            if i2applocal:
                eqerr = []
                ald = dictifyAppLocal(applocal)
                ild = dictifyAppLocal(i2applocal)
                if not deepeq(ald, ild, (), eqerr):
                    xe(
                        "{} indexer and algod disagree on app local, {}\nindexer={}\nalgod={}\n".format(
                            niceaddr, eqerr, json_pp(i2applocal), json_pp(applocal)
                        )
                    )
            else:
                xe(
                    "{} algod has app local but not indexer: {!r} ".format(
                        niceaddr, applocal
                    )
                )
        elif i2applocal:
            xe(
                "{} indexer has app local but not algod: {!r} ".format(
                    niceaddr, i2applocal
                )
            )

        # TODO: warn about unchecked fields? pop checked fields and deepeq the rest?

        with self.lock:
            if xe.ok:
                self.match += 1
            else:
                self.err.write(
                    "{} indexer {!r}\n".format(niceaddr, sorted(indexer_account.keys()))
                )
                self.neq += 1
                self.mismatches.append((address, "\n".join(xe.errors)))

    def summary(self):
        self.err.write("{} match {} neq\n".format(self.match, self.neq))


# data/basics/userBalance.go AccountData
# "onl":uint Status
# "algo":uint64 MicroAlgos


def jsonable(x, display=False):
    if x is None:
        return x
    if isinstance(x, dict):
        return {str(jsonable(k, display)): jsonable(v, display) for k, v in x.items()}
    if isinstance(x, list):
        return [jsonable(e, display) for e in x]
    if isinstance(x, bytes):
        if display:
            return "b:" + base64.b64encode(x).decode()
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
    tt = tv[b"tt"]
    if tt == 1:  # bytes
        return {"type": tt, "bytes": tv[b"tb"]}
    return {"type": tt, "uint": tv[b"ui"]}


# convert from msgpack TealKeyValue to api TealKeyValueStore
def tkvToTkvs(tkv):
    if not tkv:
        return None
    return [{"key": k, "value": tvToApiTv(v)} for k, v in tkv.items()]


# msgpack to api
def schemaDecode(sch):
    if sch is None:
        return None
    return {
        "num-uint": sch.get(b"nui", 0),
        "num-byte-slice": sch.get(b"nbs", 0),
    }


def dmaybe(d, k, v):
    if v is None:
        return
    d[k] = v


def maybeb64(x):
    if x:
        return base64.b64encode(x).decode()
    return x


def token_addr_from_args(args):
    if args.algod:
        addr = open(os.path.join(args.algod, "algod.net"), "rt").read().strip()
        token = open(os.path.join(args.algod, "algod.token"), "rt").read().strip()
    else:
        addr = args.algod_net
        token = args.algod_token
    if not addr.startswith("http"):
        addr = "http://" + addr
    return token, addr


def compare(indexer_account, i2a_checker, algod, indexer, indexer_headers):
    niceaddr = indexer_account["address"]
    if niceaddr in (reward_niceaddr, fee_niceaddr):
        return
    algod_account = algod.account_info(niceaddr)
    if algod_account["round"] != indexer_account["round"]:
        indexer_account = indexerAccountFromAddr(
            indexer,
            niceaddr,
            blockround=algod_account["round"],
            headers=indexer_headers,
        )
    i2a_checker.check(indexer_account, algod_account)


def compare_thread(ia_queue, i2a_checker, algod, indexer, indexer_headers):
    while True:
        indexer_account = ia_queue.get()
        if indexer_account is None:
            return
        compare(indexer_account, i2a_checker, algod, indexer, indexer_headers)


# decrement by one the big-endian number implicit in a byte string
# crashes on [0,0,0, ...]
def bytedec(bs):
    bs = list(bs)
    pos = len(bs) - 1
    while bs[pos] == 0:
        bs[pos] = 0xFF
        pos -= 1  # carry from previous position
    bs[pos] -= 1
    return bytes(bs)


# for --shard a/b return address range (minaddr-1, maxaddr)
# a in [1..b], e.g. --shard 1/16 .. 16/16
# may return (None, addr) or (addr, None) for first/last shard
def shard_bounds(shardspec):
    a, b = shardspec.split("/")
    a = int(a)
    b = int(b)
    chunk = (2**64) // b
    if a < 1:
        a = 1
    if a > b:
        a = b

    if a == 1:
        minsplit = None
    else:
        minsplit = struct.pack(">Q", chunk * (a - 1))
        minsplit += bytes([0] * 24)
        minsplit = bytedec(minsplit)
    if a == b:
        maxsplit = None
    else:
        maxsplit = struct.pack(">Q", chunk * a)
        maxsplit += bytes([0] * 24)
    return (minsplit, maxsplit)


def check_from_algod(args):
    global graceful_stop
    token, addr = token_addr_from_args(args)
    algod = AlgodClient(token, addr)
    if args.genesis or args.algod:
        gpath = args.genesis or os.path.join(args.algod, "genesis.json")
        getGenesisVars(gpath)
    else:
        process_genesis(algod.algod_request("GET", "/genesis"))
    status = algod.status()
    iloadstart = time.time()
    indexer_headers = None
    gtaddr = args.gtaddr
    maxaddr = None
    if args.shard:
        minaddr, maxaddr = shard_bounds(args.shard)
        if minaddr is not None:
            gtaddr = algosdk.encoding.encode_address(minaddr)
    if args.indexer_token:
        indexer_headers = {"X-Indexer-API-Token": token}

    lastlog = time.time()
    i2a_checker = CheckContext(sys.stderr)
    # for each account in indexer, get it from algod, and maybe re-get it from indexer at the round algod had
    ia_queue = queue.Queue(maxsize=10)
    threads = []
    for i in range(args.threads):
        t = threading.Thread(
            target=compare_thread,
            args=(ia_queue, i2a_checker, algod, args.indexer, indexer_headers),
        )
        t.start()
        threads.append(t)
    lastaddr = None
    for indexer_account in indexerAccounts(
        args.indexer,
        addrlist=(args.accounts and args.accounts.split(",")),
        headers=indexer_headers,
        gtaddr=gtaddr,
    ):
        lastaddr = indexer_account["address"]
        if maxaddr is not None:
            la = algosdk.encoding.decode_address(lastaddr)
            if la > maxaddr:
                break
        ia_queue.put(indexer_account)
        if graceful_stop:
            break
        now = time.time()
        if (now - lastlog) > 5:
            with i2a_checker.lock:
                logger.info(
                    "%d match, %d neq, through about %s",
                    i2a_checker.match,
                    i2a_checker.neq,
                    lastaddr,
                )
            lastlog = now
    for t in threads:
        ia_queue.put(None)  # signal end
    for t in threads:
        t.join()
    if graceful_stop:
        logger.info("last addr %s", lastaddr)
    return i2a_checker


def main():
    import argparse

    ap = argparse.ArgumentParser()
    ap.add_argument("-d", "--algod", default=None, help="algorand data dir")
    ap.add_argument("--algod-net", default=None, help="algod host:port")
    ap.add_argument("--algod-token", default=None, help="algod token")
    ap.add_argument("--genesis", default=None, help="path to genesis.json")
    ap.add_argument(
        "--limit", default=None, type=int, help="debug limit number of accoutns to dump"
    )
    ap.add_argument("--indexer", default=None, help="URL to indexer to fetch from")
    ap.add_argument("--indexer-token", default=None, help="indexer API token")
    ap.add_argument(
        "--asset", default=None, help="filter on accounts possessing asset id"
    )
    ap.add_argument(
        "--mismatches",
        default=10,
        type=int,
        help="max number of mismatches to show details on (0 for no limit)",
    )
    ap.add_argument("-q", "--quiet", default=False, action="store_true")
    ap.add_argument("--verbose", default=False, action="store_true")
    ap.add_argument("--accounts", help="comma separated list of accounts to test")
    ap.add_argument("--ssl-no-validate", default=False, action="store_true")
    ap.add_argument(
        "--threads",
        default=4,
        type=int,
        help="number of request-compare threads to run",
    )
    ap.add_argument("--gtaddr", default=None, help="fetch accounts after this addr")
    ap.add_argument("--shard", default=None, help="a/b for a in [1..b]")
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    if not args.indexer:
        logger.error("need --indexer to specify root url of indexer api")
        return 1
    if (not args.algod) and ((not args.algod_net) or (not args.algod_token)):
        logger.error("need --algod datadir or --algod-net and --algod-token")
        return 1
    if args.ssl_no_validate:
        global ssl_no_validate
        ssl_no_validate = (
            SSLContext()
        )  # empty context doesn't validate; use for self-signed certs
    if args.gtaddr and args.shard:
        logger.error("--gtaddr and --shard are incompatible")
        return 1
    out = sys.stdout
    err = sys.stderr

    i2a_checker = check_from_algod(args)
    retval = 0
    i2a_checker.summary()
    mismatches = i2a_checker.mismatches
    if args.mismatches and len(mismatches) > args.mismatches:
        mismatches = mismatches[: args.mismatches]
    for addr, msg in mismatches:
        niceaddr = algosdk.encoding.encode_address(addr)
        if addr in (reward_addr, fee_addr):
            logger.debug("skip %s", niceaddr)
            # we know accounting for the special addrs is different
            continue
        retval = 1
        xaddr = base64.b16encode(addr).decode().lower()
        err.write("\n{} '\\x{}'\n\t{}\n".format(niceaddr, xaddr, msg))
        tcount = 0
        tmore = 0
        for txn in indexerAccountTxns(
            args.indexer, addr, limit=30, token=args.indexer_token
        ):
            if tcount > 10:
                tmore += 1
                continue
            tcount += 1
            sender = txn.pop("sender")
            cround = txn.pop("confirmed-round")
            intra = txn.pop("intra-round-offset")
            parts = ["{}:{}".format(cround, intra), "s={}".format(sender)]
            pay = txn.get("payment-transaction", None)
            if pay:
                receiver = pay.pop("receiver", None)
                closeto = pay.pop("close-remainder-to", None)
            else:
                receiver = None
                closeto = None
            axfer = txn.get("asset-transfer-transaction", None)
            if axfer is not None:
                asnd = axfer.pop("sender", None)
                arcv = axfer.pop("receiver", None)
                aclose = axfer.pop("close-to", None)
            else:
                asnd = None
                arcv = None
                aclose = None
            if asnd:
                parts.append("as={}".format(asnd))
            if receiver:
                parts.append("r={}".format(receiver))
            if closeto:
                parts.append("c={}".format(closeto))
            if arcv:
                parts.append("ar={}".format(arcv))
            if aclose:
                parts.append("ac={}".format(aclose))
            # everything else
            parts.append(json.dumps(txn))
            err.write(" ".join(parts) + "\n")
        if tmore:
            err.write("... and {} more\n".format(tmore))
    if retval == 0:
        logger.info("validate_accounting OK")
    return retval


if __name__ == "__main__":
    sys.exit(main())
