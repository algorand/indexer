#!/usr/bin/env python3
#

import atexit
from datetime import datetime, timedelta
import glob
import gzip
import io
import logging
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time

import yaml

from e2e_common.util import find_binary
import e2e_conduit.fixtures.importers as importers
import e2e_conduit.fixtures.processors as processors
import e2e_conduit.fixtures.exporters as exporters

logger = logging.getLogger(__name__)


def main():
    start = time.time()
    import argparse

    ap = argparse.ArgumentParser()
    # TODO FIXME convert keep_temps to debug mode which will leave all resources running/around
    # So files will not be deleted and docker containers will be left running
    ap.add_argument("--keep-temps", default=False, action="store_true")
    ap.add_argument(
        "--conduit-bin",
        default=None,
        help="path to conduit binary, otherwise search PATH",
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
    conduit_bin = find_binary(args.conduit_bin, binary_name="conduit")
    sourcenet = args.source_net
    source_is_tar = False
    if not sourcenet:
        e2edata = os.getenv("E2EDATA")
        sourcenet = e2edata and os.path.join(e2edata, "net")
    if not (sourcenet or args.s3_source_net):
        raise Exception("Must provide either local or s3 network to run test against")

    # Setup tempdir for conduit data dir
    tempdir = tempfile.mkdtemp()
    if not args.keep_temps:
        atexit.register(shutil.rmtree, tempdir, onerror=logger.error)
    else:
        logger.info("leaving temp dir %r", tempdir)
    # Setup algod importer
    importer_source = sourcenet if sourcenet else args.s3_source_net
    algod_importer = importers.AlgodImporter()
    algod_importer.setup(tempdir, importer_source)
    algod_importer.resolve_config_input()
    algod_importer.resolve_config_output()

    # Setup block_evaluator processor
    block_evaluator = processors.BlockEvaluator()
    block_evaluator.setup(
        algod_importer.config_output["algoddir"],
        algod_importer.config_output["token"],
        algod_importer.config_output["netaddr"],
        tempdir,
        catchpoint="",
    )
    block_evaluator.resolve_config_input()
    block_evaluator.resolve_config_output()
    # Setup postgresql exporter
    pgsql_exporter = exporters.PostgresqlExporter()
    pgsql_exporter.setup()
    pgsql_exporter.resolve_config_input()
    pgsql_exporter.resolve_config_output()

    # Write conduit config to data directory
    with open(os.path.join(tempdir, "conduit.yml"), "w") as conduit_cfg:
        yaml.dump(
            {
                "LogLevel": "info",
                "Importer": {
                    "Name": algod_importer.name,
                    "Config": algod_importer.config_input,
                },
                "Processors": [
                    {
                        "Name": block_evaluator.name,
                        "Config": block_evaluator.config_input,
                    }
                ],
                "Exporter": {
                    "Name": pgsql_exporter.name,
                    "Config": pgsql_exporter.config_input,
                },
            },
            conduit_cfg,
        )

    # Run conduit
    cmd = [conduit_bin, "-d", tempdir]
    logger.debug("%s", " ".join(map(repr, cmd)))
    indexerdp = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    atexit.register(indexerdp.kill)
    indexerout = subslurp(indexerdp.stdout)

    logger.info(f"Waiting for conduit to reach round {algod_importer.lastblock}")

    try:
        indexerout.run(algod_importer.lastblock)
    except RuntimeError as exc:
        logger.error(f"{exc}")
        logger.error(f"conduit hit an error during execution: {indexerout.error_log}")
        sys.stderr.write(indexerout.dump())
        return 1

    if indexerout.round >= algod_importer.lastblock:
        logger.info("reached expected round={}".format(algod_importer.lastblock))
        dt = time.time() - start
        sys.stdout.write("conduit e2etest OK ({:.1f}s)\n".format(dt))
        return 0
    logger.error("conduit did not reach round={}".format(algod_importer.lastblock))
    sys.stderr.write(indexerout.dump())
    return 1


class subslurp:
    # accumulate stdout or stderr from a subprocess and hold it for debugging if something goes wrong
    def __init__(self, f):
        self.f = f
        self.buf = io.BytesIO()
        self.gz = gzip.open(self.buf, "wb")
        self.timeout = timedelta(seconds=120)
        # Matches conduit log output: "Pipeline round: 110"
        self.round_re = re.compile(b'.*"Pipeline round: ([0-9]+)"')
        self.round = 0
        self.error_log = None

    def logIsError(self, log_line):
        if b"error" in log_line:
            self.error_log = log_line
            return True
        return False

    def tryParseRound(self, log_line):
        m = self.round_re.match(log_line)
        if m is not None and m.group(1) is not None:
            self.round = int(m.group(1))

    def run(self, lastround):
        if len(self.f.peek().strip()) == 0:
            logger.info("No Conduit output found")
            return

        start = datetime.now()
        lastlog = datetime.now()
        while (
            datetime.now() - start < self.timeout
            and datetime.now() - lastlog < timedelta(seconds=15)
        ):
            for line in self.f:
                logger.info(line)
                lastlog = datetime.now()
                if self.gz is not None:
                    self.gz.write(line)
                self.tryParseRound(line)
                if self.round >= lastround:
                    logger.info(f"Conduit reached desired lastround: {lastround}")
                    return
                if self.logIsError(line):
                    raise RuntimeError(f"E2E tests logged an error: {self.error_log}")

    def dump(self):
        self.gz.close()
        self.gz = None
        self.buf.seek(0)
        r = gzip.open(self.buf, "rt")
        return r.read()


if __name__ == "__main__":
    sys.exit(main())
