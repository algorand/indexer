#!/usr/bin/env python3
import atexit
import glob
import gzip
import io
import json
import logging
import os
import random
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

from e2e_common.util import (
    xrun,
    atexitrun,
    find_binary,
    ensure_test_db,
    firstFromS3Prefix,
    hassuffix,
    countblocks,
)

logger = logging.getLogger(__name__)


def main():
    start = time.time()
    import argparse

    ap = argparse.ArgumentParser()
    ap.add_argument("--keep-temps", default=False, action="store_true")
    # keep alive is convenient in docker environments to prevent exiting after an error
    ap.add_argument("--keep-alive", default=False, action="store_true")
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
        "--conduit-bin",
        default=None,
        help="path to the conduit binary, otherwise search PATH"
    )
    ap.add_argument(
        "--conduit-dir",
        default=None,
        help="Path to the directory in which to run conduit. Required when not read-only."
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
    ap.add_argument(
        "--read-only",
        default=False,
        type=bool,
        help="Run the indexer daemon in read-only mode.",
    )
    ap.add_argument("--verbose", default=False, action="store_true")
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    indexer_bin = find_binary(args.indexer_bin)
    conduit_bin = find_binary(args.conduit_bin, binary_name="conduit")
    tempdir = tempfile.mkdtemp()
    if not args.keep_temps:
        atexit.register(shutil.rmtree, tempdir, onerror=logger.error)
    else:
        logger.info("leaving temp dir %r", tempdir)
    # Sane default for lastblock--used for read_only runs
    lastblock = 93
    algoddir = None
    if args.keep_alive:
        # register after keep_temps so that the temp files are still there.
        atexitrun(["sleep", "infinity"])
    psqlstring = ensure_test_db(args.connection_string, args.keep_temps)
    aiport = args.indexer_port or random.randint(4000, 30000)
    cmd = [
        indexer_bin,
        "daemon",
        "-P",
        psqlstring,
        "--dev-mode",
        "--server",
        ":{}".format(aiport),
    ]
    conduitout = None
    if not args.read_only:
        if not args.conduit_dir:
            raise Exception("Must provide --conduit-dir when not readonly")
        tempnet, lastblock = setup_algod(tempdir, args.source_net, args.s3_source_net)
        algoddir = os.path.join(tempnet, "Node")
        with open(os.path.join(algoddir, "algod.token"), "r") as token_file:
            algod_token = token_file.read()
        with open(os.path.join(algoddir, "algod.net"), "r") as net_file:
            algod_net = net_file.read()
        cfg = None
        with open(os.path.join(args.conduit_dir, "conduit.yml"), "r") as conduit_cfg:
            cfg = conduit_cfg.read()
        cfg = cfg.format(ALGOD_ADDR=algod_net, ALGOD_TOKEN=algod_token, CONNECTION_STRING=psqlstring)
        with open(os.path.join(args.conduit_dir, "conduit.yml"), "w") as conduit_cfg:
            conduit_cfg.write(cfg)
        conduitout = run_conduit(conduit_bin, args.conduit_dir, algod_token, algod_net)
        time.sleep(5)

    logger.debug("%s", " ".join(map(repr, cmd)))
    indexerdp = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    indexerout = subslurp(indexerdp.stdout)
    indexerout.start()
    atexit.register(indexerdp.kill)
    time.sleep(0.2)

    indexerurl = "http://localhost:{}/".format(aiport)
    healthurl = indexerurl + "health"
    for attempt in range(20):
        (ok, healthJson) = tryhealthurl(healthurl, args.verbose, waitforround=lastblock)
        if ok:
            logger.debug("health round={} OK".format(lastblock))
            break
        time.sleep(0.5)
    if not ok:
        logger.error(
            "could not get indexer health, or did not reach round={}\n{}".format(
                lastblock, healthJson
            )
        )
        if conduitout:
            sys.stderr.write(conduitout.dump())
        sys.stderr.write(indexerout.dump())
        return 1
    try:
        logger.info("reached expected round={}".format(lastblock))
        if not args.read_only:
            xrun(
                [
                    "validate-accounting",
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
        if conduitout:
            sys.stderr.write(conduitout.dump())
        sys.stderr.write(indexerout.dump())
        raise
    dt = time.time() - start
    sys.stdout.write("indexer e2etest OK ({:.1f}s)\n".format(dt))

    return 0


def setup_algod(tempdir, sourcenet, s3_source_net):
    source_is_tar = False
    if not sourcenet:
        e2edata = os.getenv("E2EDATA")
        sourcenet = e2edata and os.path.join(e2edata, "net")
    if sourcenet and hassuffix(sourcenet, ".tar", ".tar.gz", ".tar.bz2", ".tar.xz"):
        source_is_tar = True
    if not (source_is_tar or (sourcenet and os.path.isdir(sourcenet))):
        tarname = s3_source_net
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
        prefix = "indexer/e2e4"
        if "/" in tarname:
            cmhash_tarnme = tarname.split("/")
            cmhash = cmhash_tarnme[0]
            tarname =cmhash_tarnme[1]
            prefix+="/"+cmhash
            tarpath = os.path.join(tempdir, tarname)
        else:
            tarpath = os.path.join(tempdir, tarname)

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
    # Reset the secondary node, and enable follow mode.
    # This is what conduit will connect to for data access.
    for root, dirs, files in os.walk(os.path.join(tempnet, 'Node', 'tbd-v1')):
        for f in files:
            if ".sqlite" in f:
                os.remove(os.path.join(root, f))
    cf = {}
    with open(os.path.join(tempnet, "Node", "config.json"), "r") as config_file:
        cf = json.load(config_file)
        cf['EnableFollowMode'] = True
    with open(os.path.join(tempnet, "Node", "config.json"), "w") as config_file:
        config_file.write(json.dumps(cf))
    try:
        xrun(["goal", "network", "start", "-r", tempnet])
    except Exception:
        logger.error("failed to start private network, looking for node.log")
        found = False
        log_files = ("node.log", "algod-err.log",)
        for root, dirs, files in os.walk(tempnet):
            for f in files:
                if f in log_files:
                    found = True
                    p = os.path.join(root, f)
                    logger.error("Found {}: {}".format(f, p))
                    with open(p) as nf:
                        for line in nf:
                            logger.error("   {}".format(line))
        if found is False:
            logger.error("Unable to find log file")
        raise

    atexitrun(["goal", "network", "stop", "-r", tempnet])
    return tempnet, lastblock


def tryhealthurl(healthurl, verbose=False, waitforround=200):
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


def run_conduit(conduit_binary, conduit_dir, algod_token, algod_net):
    conduitcmd = [conduit_binary, "-d", conduit_dir,]
    conduitproc = subprocess.Popen(conduitcmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    conduitout = subslurp(conduitproc.stdout)
    conduitout.start()
    atexit.register(conduitproc.kill)
    return conduitout


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
