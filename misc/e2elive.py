#!/usr/bin/env python3
#

import atexit
import glob
import gzip
import io
import json
import logging
import os
import random
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

from util import xrun, atexitrun, find_indexer, ensure_test_db, firstFromS3Prefix

logger = logging.getLogger(__name__)


def main():
    start = time.time()
    import argparse

    ap = argparse.ArgumentParser()
    ap.add_argument("--keep-temps", default=False, action="store_true")
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
    ap.add_argument(
        "--source-net",
        help="Path to test network directory containing Primary and other nodes. May be a tar file.",
    )
    ap.add_argument(
        "--s3-source-net",
        help="AWS S3 key suffix to test network tarball containing Primary and other nodes. Must be a tar bz2 file.",
    )
    ap.add_argument("--verbose", default=False, action="store_true")
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    indexer_bin = find_indexer(args.indexer_bin)
    sourcenet = args.source_net
    source_is_tar = False
    if not sourcenet:
        e2edata = os.getenv("E2EDATA")
        sourcenet = e2edata and os.path.join(e2edata, "net")
    if sourcenet and hassuffix(sourcenet, ".tar", ".tar.gz", ".tar.bz2", ".tar.xz"):
        source_is_tar = True
    tempdir = tempfile.mkdtemp()
    if not args.keep_temps:
        atexit.register(shutil.rmtree, tempdir, onerror=logger.error)
    else:
        logger.info("leaving temp dir %r", tempdir)
    if not (source_is_tar or (sourcenet and os.path.isdir(sourcenet))):
        tarname = args.s3_source_net
        if not tarname:
            raise Exception(
                "Must provide either local or s3 network to run test against"
            )
        tarname = f"{tarname}.tar.bz2"

        # fetch test data from S3
        bucket = "algorand-testdata"
        import boto3
        from botocore.config import Config
        from botocore import UNSIGNED

        s3 = boto3.client("s3", config=Config(signature_version=UNSIGNED))
        tarpath = os.path.join(tempdir, tarname)
        prefix = "indexer/e2e4"
        success = firstFromS3Prefix(s3, bucket, prefix, tarname, outpath=tarpath)
        if not success:
            raise Exception(
                f"failed to locate tarname={tarname} from AWS S3 path {bucket}/{prefix}"
            )
        source_is_tar = True
        sourcenet = tarpath
    tempnet = os.path.join(tempdir, "net")
    if source_is_tar:
        xrun(["tar", "-C", tempdir, "-x", "-f", sourcenet])
    else:
        xrun(["rsync", "-a", sourcenet + "/", tempnet + "/"])
    blockfiles = glob.glob(
        os.path.join(tempdir, "net", "Primary", "*", "*.block.sqlite")
    )
    lastblock = countblocks(blockfiles[0])
    try:
        xrun(["goal", "network", "start", "-r", tempnet])
    except Exception:
        logger.error("failed to start private network, looking for node.log")
        for root, dirs, files in os.walk(tempnet):
            for f in files:
                if f == "node.log":
                    p = os.path.join(root, f)
                    logger.error("found node.log: {}".format(p))
                    with open(p) as nf:
                        for line in nf:
                            logger.error("   {}".format(line))
        raise

    atexitrun(["goal", "network", "stop", "-r", tempnet])

    psqlstring = ensure_test_db(args.connection_string, args.keep_temps)
    algoddir = os.path.join(tempnet, "Primary")
    aiport = args.indexer_port or random.randint(4000, 30000)
    cmd = [
        indexer_bin,
        "daemon",
        "--data-dir",
        tempdir,
        "-P",
        psqlstring,
        "--dev-mode",
        "--algod",
        algoddir,
        "--server",
        ":{}".format(aiport),
    ]
    logger.debug("%s", " ".join(map(repr, cmd)))
    indexerdp = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    indexerout = subslurp(indexerdp.stdout)
    indexerout.start()
    atexit.register(indexerdp.kill)
    time.sleep(0.2)

    indexerurl = "http://localhost:{}/".format(aiport)
    healthurl = indexerurl + "health"
    for attempt in range(20):
        (ok, json) = tryhealthurl(healthurl, args.verbose, waitforround=lastblock)
        if ok:
            logger.debug("health round={} OK".format(lastblock))
            break
        time.sleep(0.5)
    if not ok:
        logger.error(
            "could not get indexer health, or did not reach round={}\n{}".format(
                lastblock, json
            )
        )
        sys.stderr.write(indexerout.dump())
        return 1
    try:
        logger.info("reached expected round={}".format(lastblock))
        xrun(
            [
                "python3",
                "misc/validate_accounting.py",
                "--verbose",
                "--algod",
                algoddir,
                "--indexer",
                indexerurl,
            ],
            timeout=20,
        )
        xrun(
            ["go", "run", "cmd/e2equeries/main.go", "-pg", psqlstring, "-q"], timeout=15
        )
    except Exception:
        sys.stderr.write(indexerout.dump())
        raise
    dt = time.time() - start
    sys.stdout.write("indexer e2etest OK ({:.1f}s)\n".format(dt))

    return 0


def hassuffix(x, *suffixes):
    for s in suffixes:
        if x.endswith(s):
            return True
    return False


def countblocks(path):
    db = sqlite3.connect(path)
    cursor = db.cursor()
    cursor.execute("SELECT max(rnd) FROM blocks")
    row = cursor.fetchone()
    cursor.close()
    db.close()
    return row[0]


def tryhealthurl(healthurl, verbose=False, waitforround=100):
    try:
        response = urllib.request.urlopen(healthurl)
        if response.code != 200:
            return (False, "")
        raw = response.read()
        logger.debug("health %r", raw)
        ob = json.loads(raw)
        rt = ob.get("message")
        if not rt:
            return (False, raw)
        return (int(rt) >= waitforround, raw)
    except Exception as e:
        if verbose:
            logging.warning("GET %s %s", healthurl, e)
        return (False, "")


class subslurp:
    # asynchronously accumulate stdout or stderr from a subprocess and hold it for debugging if something goes wrong
    def __init__(self, f):
        self.f = f
        self.buf = io.BytesIO()
        self.gz = gzip.open(self.buf, "wb")
        self.l = threading.Lock()
        self.t = None

    def run(self):
        for line in self.f:
            with self.l:
                if self.gz is None:
                    return
                self.gz.write(line)

    def dump(self):
        with self.l:
            self.gz.close()
            self.gz = None
        self.buf.seek(0)
        r = gzip.open(self.buf, "rt")
        return r.read()

    def start(self):
        self.t = threading.Thread(target=self.run)
        self.t.daemon = True
        self.t.start()


if __name__ == "__main__":
    sys.exit(main())
