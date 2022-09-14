#!/usr/bin/env python3
#

import argparse
import logging
import os
import sys

from e2e_common.util import find_binary
import e2e_conduit.fixtures.importers as importers
import e2e_conduit.fixtures.processors as processors
import e2e_conduit.fixtures.exporters as exporters
from e2e_conduit.runner import ConduitE2ETestRunner
from e2e_conduit.scenarios import scenarios
from e2e_conduit.scenarios.indexer_scenario import indexer_scenario

logger = logging.getLogger(__name__)


def main():
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
    sourcenet = args.source_net
    source_is_tar = False
    if not sourcenet:
        e2edata = os.getenv("E2EDATA")
        sourcenet = e2edata and os.path.join(e2edata, "net")
    importer_source = sourcenet if sourcenet else args.s3_source_net
    if importer_source:
        scenarios.append(indexer_scenario(importer_source))

    runner = ConduitE2ETestRunner(args.conduit_bin, keep_temps=args.keep_temps)

    success = True
    for scenario in scenarios:
        runner.setup_scenario(scenario)
        if runner.run_scenario(scenario) != 0:
            success = False
    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
