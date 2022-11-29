#!/usr/bin/env python3
#
# usage:
#  python3 misc/liveindextest.py
#
# Requires go-algorand to be checked out on GOPATH.
# Requires local postgresql and `createdb` `dropdb` standard utils.
# `goal` etc should be built on PATH
# `algorand-indexer` can be installed on PATH or at its development location from `make` or `go build` at cmd/algorand-indexer/algorand-indexer
# pip install py-algorand-sdk
#
# The Test:
# Create a local private Algorand network
# Create a temporary postgres database for indexer
# Run indexer following the primary algod
# Submit a txn using py-algorand-sdk
# Checks that indexer reports that txn by searching for it by txid.
#
# Runs in about 30 seconds on my macbook

import atexit
import base64
import glob
import logging
import os
import random
import shutil
import subprocess
import sys
import tempfile
import threading
import time

import algosdk
import algosdk.v2client

from e2e_common.util import xrun, atexitrun, find_indexer, ensure_test_db

logger = logging.getLogger(__name__)


def find_go_algorand():
    gopath = os.getenv("GOPATH")
    for path in gopath.split(":"):
        goa = os.path.join(path, "src", "github.com", "algorand", "go-algorand")
        if os.path.isdir(goa):
            return goa
    return None


already_stopped = False
already_deleted = False


def goal_network_stop(netdir, normal_cleanup=False):
    global already_stopped, already_deleted
    if already_stopped or already_deleted:
        return

    logger.info("stop network in %s", netdir)
    try:
        xrun(["goal", "network", "stop", "-r", netdir], timeout=10)
    except Exception as e:
        logger.error("error stopping network", exc_info=True)
        if normal_cleanup:
            raise e
    already_stopped = True


def openkmd(algodata):
    kmdnetpath = sorted(glob.glob(os.path.join(algodata, "kmd-*", "kmd.net")))[-1]
    kmdnet = open(kmdnetpath, "rt").read().strip()
    kmdtokenpath = sorted(glob.glob(os.path.join(algodata, "kmd-*", "kmd.token")))[-1]
    kmdtoken = open(kmdtokenpath, "rt").read().strip()
    kmd = algosdk.kmd.KMDClient(kmdtoken, "http://" + kmdnet)
    return kmd


def openalgod(algodata):
    algodnetpath = os.path.join(algodata, "algod.net")
    algodnet = open(algodnetpath, "rt").read().strip()
    algodtokenpath = os.path.join(algodata, "algod.token")
    algodtoken = open(algodtokenpath, "rt").read().strip()
    algod = algosdk.algod.AlgodClient(algodtoken, "http://" + algodnet)
    return algod


class RunContext:
    def __init__(self, env):
        self.env = env
        self.kmd = None
        self.algod = None
        self.lock = threading.Lock()
        self.pubw = None
        self.maxpubaddr = None

    def connect(self):
        with self.lock:
            self._connect()
            return self.algod, self.kmd

    def _connect(self):
        if self.algod and self.kmd:
            return
        # should run from inside self.lock
        xrun(["goal", "kmd", "start", "-t", "200"], env=self.env, timeout=5)
        algodata = self.env["ALGORAND_DATA"]
        self.kmd = openkmd(algodata)
        self.algod = openalgod(algodata)

    def get_pub_wallet(self):
        with self.lock:
            self._connect()
            if not (self.pubw and self.maxpubaddr):
                # find private test node public wallet and its richest account
                wallets = self.kmd.list_wallets()
                pubwid = None
                for xw in wallets:
                    if xw["name"] == "unencrypted-default-wallet":
                        pubwid = xw["id"]
                pubw = self.kmd.init_wallet_handle(pubwid, "")
                pubaddrs = self.kmd.list_keys(pubw)
                pubbalances = []
                maxamount = 0
                maxpubaddr = None
                for pa in pubaddrs:
                    pai = self.algod.account_info(pa)
                    if pai["amount"] > maxamount:
                        maxamount = pai["amount"]
                        maxpubaddr = pai["address"]
                self.pubw = pubw
                self.maxpubaddr = maxpubaddr
            return self.pubw, self.maxpubaddr

    def do_txn(self):
        pubw, maxpubaddr = self.get_pub_wallet()
        algod, kmd = self.connect()

        # create a wallet with an addr to send to
        walletname = base64.b16encode(os.urandom(16)).decode()
        winfo = kmd.create_wallet(walletname, "")
        handle = kmd.init_wallet_handle(winfo["id"], "")
        addr = kmd.generate_key(handle)

        # send one million Algos to the test wallet's account
        params = algod.suggested_params()
        round = params["lastRound"]
        txn = algosdk.transaction.PaymentTxn(
            sender=maxpubaddr,
            fee=params["minFee"],
            first=round,
            last=round + 100,
            gh=params["genesishashb64"],
            receiver=addr,
            amt=1000000000000,
            flat_fee=True,
        )
        stxn = kmd.sign_transaction(pubw, "", txn)
        txid = algod.send_transaction(stxn)
        for i in range(50):
            txinfo = algod.pending_transaction_info(txid)
            if txinfo.get("round"):
                break
            time.sleep(0.1)
        return txid, txinfo


def main():
    start = time.time()
    import argparse

    ap = argparse.ArgumentParser()
    ap.add_argument("--go-algorand", help="path to go-algorand checkout")
    ap.add_argument(
        "--keep-temps",
        default=False,
        action="store_true",
        help="if set, keep all the test files",
    )
    ap.add_argument(
        "--indexer-bin",
        default=None,
        help="path to algorand-indexer binary, otherwise search PATH",
    )
    ap.add_argument(
        "--indexer-port",
        default=None,
        type=int,
        help="port to run indexer on. defaults to random in [4000,30000]",
    )
    ap.add_argument(
        "--connection-string",
        help="Use this connection string instead of attempting to manage a local database.",
    )
    ap.add_argument("--verbose", default=False, action="store_true")
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    indexer_bin = find_indexer(args.indexer_bin)
    goalgorand = args.go_algorand or find_go_algorand()

    # env for child processes
    env = dict(os.environ)

    tempdir = os.getenv("TEMPDIR")
    if not tempdir:
        tempdir = tempfile.mkdtemp()
        env["TEMPDIR"] = tempdir
        logger.info("created TEMPDIR %r", tempdir)
        if not args.keep_temps:
            # If we created a tmpdir and we're not keeping it, clean it up.
            # If an outer process specified $TEMPDIR, let them clean it up.
            atexit.register(shutil.rmtree, tempdir, onerror=logger.error)
        else:
            atexit.register(
                print, "keeping temps. to clean up:\nrm -rf {}".format(tempdir)
            )

    netdir = os.path.join(tempdir, "net")
    env["NETDIR"] = netdir

    template = os.path.join(
        goalgorand, "test/testdata/nettemplates/TwoNodes50EachFuture.json"
    )
    xrun(
        ["goal", "network", "create", "-r", netdir, "-n", "tbd", "-t", template],
        timeout=30,
    )
    xrun(["goal", "network", "start", "-r", netdir], timeout=30)
    atexit.register(goal_network_stop, netdir)

    algodata = os.path.join(netdir, "Node")
    env["ALGORAND_DATA"] = algodata

    psqlstring = ensure_test_db(args.connection_string, args.keep_temps)
    primary = os.path.join(netdir, "Primary")
    aiport = args.indexer_port or random.randint(4000, 30000)
    indexer_token = "security-theater"
    indexerp = subprocess.Popen(
        [
            indexer_bin,
            "daemon",
            "--algod",
            primary,
            "--postgres",
            psqlstring,
            "--dev-mode",
            "--server",
            ":{}".format(aiport),
            "--token",
            indexer_token,
        ]
    )
    atexit.register(indexerp.kill)

    rc = RunContext(env)
    txid, txinfo = rc.do_txn()
    logger.debug("submitted txid %s, %r", txid, txinfo)

    indexer = algosdk.v2client.indexer.IndexerClient(
        indexer_token, "http://localhost:{}".format(aiport)
    )
    ok = False
    retcode = 1
    for i in range(30):
        result = indexer.search_transactions(txid=txid)
        logger.debug("seacrh_transactions: %r", result)
        they = result.get("transactions")
        if they and they[0].get("confirmed-round"):
            logger.info("OK: Got txn")
            ok = True
            retcode = 0
            break
        time.sleep(1.0)

    dt = time.time() - start
    ok = (ok and "OK") or "FAIL"
    sys.stdout.write("indexer live test {} ({:.1f}s)\n".format(ok, dt))
    return retcode


if __name__ == "__main__":
    sys.exit(main())
