import argparse
import json
import logging
import os
import sys

import boto3
from botocore.config import Config
from botocore import UNSIGNED

from e2e_common.util import (
    xrun,
    atexitrun,
    firstFromS3Prefix,
    hassuffix,
)

logger = logging.getLogger(__name__)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--s3-source-net",
        help="AWS S3 key suffix to test network tarball containing Primary and other nodes. Must be a tar bz2 file.",
    )
    ap.add_argument(
        "--algod-dir",
        help="Directory to run algod network in.",
    )
    ap.add_argument("--verbose", default=False, action="store_true")
    args = ap.parse_args()
    if not args.s3_source_net:
        raise Exception("Must provide --s3-source-net")
    if not args.algod_dir:
        raise Exception("Must provide --algod-dir")

    tarname = f"{args.s3_source_net}.tar.bz2"
     # fetch test data from S3
    bucket = "algorand-testdata"
    s3 = boto3.client("s3", config=Config(signature_version=UNSIGNED))
    prefix = "indexer/e2e4"
    if "/" in tarname:
        cmhash_tarnme = tarname.split("/")
        cmhash = cmhash_tarnme[0]
        tarname =cmhash_tarnme[1]
        prefix+="/"+cmhash
        tarpath = os.path.join(args.algod_dir, tarname)
    else:
        tarpath = os.path.join(args.algod_dir, tarname)
    success = firstFromS3Prefix(s3, bucket, prefix, tarname, outpath=tarpath)
    if not success:
        raise Exception(f"failed to locate tarname={tarname} from AWS S3 path {bucket}/{prefix}")
    sourcenet = tarpath
    tempnet = os.path.join(args.algod_dir, "net")
    xrun(["tar", "-C", args.algod_dir, "-x", "-f", sourcenet])

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

    return 0

if __name__ == "__main__":
    sys.exit(main())
