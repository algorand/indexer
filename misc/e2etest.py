#!/usr/bin/env python3
#
# pip install "msgpack >=1" py-algorand-sdk boto3
# boto3 required if fetching test data from s3

import atexit
import glob
import logging
import os
import random
import subprocess
import sys
import tempfile
import time

from util import xrun, atexitrun, find_indexer, ensure_test_db

logger = logging.getLogger(__name__)

def ensureTestData(e2edata):
    blocktars = glob.glob(os.path.join(e2edata, 'blocktars', '*.tar.bz2'))
    if not blocktars:
        tarname = 'e2edata.tar.bz2'
        tarpath = os.path.join(e2edata, tarname)
        if not os.path.exists(tarpath):
            logger.info('fetching testdata from s3...')
            if not os.path.isdir(e2edata):
                os.makedirs(e2edata)
            bucket = 'algorand-testdata'
            import boto3
            from botocore.config import Config
            from botocore import UNSIGNED
            s3 = boto3.client('s3', config=Config(signature_version=UNSIGNED))
            response = s3.list_objects_v2(Bucket=bucket, Prefix='indexer/e2e1', MaxKeys=2)
            if (not response.get('KeyCount')) or ('Contents' not in response):
                logger.error('no testdata found in s3')
                sys.exit(1)
            for x in response['Contents']:
                path = x['Key']
                _, fname = path.rsplit('/', 1)
                if fname == tarname:
                    logger.info('s3://%s/%s -> %s', bucket, x['Key'], tarpath)
                    s3.download_file(bucket, x['Key'], tarpath)
                    break
        logger.info('unpacking %s', tarpath)
        subprocess.run(['tar', '-jxf', tarpath], cwd=e2edata).check_returncode()



def main():
    start = time.time()
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--keep-temps', default=False, action='store_true')
    ap.add_argument('--verbose', default=False, action='store_true')
    ap.add_argument('--connection-string', help='Use this connection string instead of attempting to manage a local database.')
    ap.add_argument('--indexer-port', default=None, type=int, help='port to run indexer on. defaults to random in [4000,30000]')
    ap.add_argument('--indexer-bin', default=None, help='path to algorand-indexer binary, otherwise search PATH')
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    indexer_bin = find_indexer(args.indexer_bin)

    e2edata = os.getenv('E2EDATA')
    if not e2edata:
        tdir = tempfile.TemporaryDirectory()
        e2edata = tdir.name
        if not args.keep_temps:
            atexit.register(tdir.cleanup)
        else:
            logger.info("leaving temp dir %r", tdir.name)
    ensureTestData(e2edata)

    psqlstring = ensure_test_db(args.connection_string, args.keep_temps)

    xrun([indexer_bin, 'import', '-P', psqlstring, os.path.join(e2edata, 'blocktars', '*'), '--genesis', os.path.join(e2edata, 'algod', 'genesis.json')], timeout=20)
    aiport = args.indexer_port or random.randint(4000,30000)
    cmd = [indexer_bin, 'daemon', '-P', psqlstring, '--dev-mode', '--no-algod', '--server', ':{}'.format(aiport)]
    logger.debug("%s", ' '.join(map(repr,cmd)))
    indexerdp = subprocess.Popen(cmd)
    atexit.register(indexerdp.kill)
    time.sleep(0.2)
    sqliteglob = os.path.join(e2edata, 'algod', '*', 'ledger.tracker.sqlite')
    sqlitepaths = glob.glob(sqliteglob)
    sqlitepath = sqlitepaths[0]
    xrun(['python3', 'misc/validate_accounting.py', '--verbose', '--dbfile', sqlitepath, '--indexer', 'http://localhost:{}/'.format(aiport)], timeout=20)
    xrun(['go', 'run', 'cmd/e2equeries/main.go', '-pg', psqlstring, '-q'], timeout=15)
    dt = time.time() - start
    sys.stdout.write("indexer e2etest OK ({:.1f}s)\n".format(dt))
    return 0

if __name__ == '__main__':
    sys.exit(main())
