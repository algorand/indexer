#!/usr/bin/env python3
#

import atexit
import glob
import json
import logging
import os
import random
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.request

from util import xrun, atexitrun, find_indexer, ensure_test_db
from e2etest import ensureTestData

logger = logging.getLogger(__name__)

def main():
    start = time.time()
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--keep-temps', default=False, action='store_true')
    ap.add_argument('--indexer-bin', default=None, help='path to algorand-indexer binary, otherwise search PATH')
    ap.add_argument('--indexer-port', default=None, type=int, help='port to run indexer on. defaults to random in [4000,30000]')
    ap.add_argument('--connection-string', help='Use this connection string instead of attempting to manage a local database.')
    ap.add_argument('--source-net', help='Path to test network directory containing Primary and other nodes. May be a tar file.')
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    indexer_bin = find_indexer(args.indexer_bin)
    sourcenet = args.source_net
    source_is_tar = False
    if not sourcenet:
        e2edata = os.getenv('E2EDATA')
        sourcenet = os.path.join(e2edata, 'net')
    if hassuffix(sourcenet, '.tar', '.tar.gz', '.tar.bz2', '.tar.xz'):
        source_is_tar = True
    if not (source_is_tar or os.path.isdir(sourcenet)):
        logger.error('need network or tar at %r', sourcenet)
        return 1
    tdir = tempfile.TemporaryDirectory()
    tempnet = os.path.join(tdir.name, 'net')
    if not args.keep_temps:
        atexit.register(tdir.cleanup)
    else:
        logger.info("leaving temp dir %r", tdir.name)
    if source_is_tar:
        xrun(['tar', '-C', tdir.name, '-x', '-f', sourcenet])
    else:
        xrun(['rsync', '-a', sourcenet + '/', tempnet + '/'])
    blockfiles = glob.glob(os.path.join(tdir.name, 'net', 'Primary', '*', '*.block.sqlite'))
    lastblock = countblocks(blockfiles[0])
    #subprocess.run(['find', tempnet, '-type', 'f'])
    xrun(['goal', 'network', 'start', '-r', tempnet])
    atexitrun(['goal', 'network', 'stop', '-r', tempnet])

    psqlstring = ensure_test_db(args.connection_string, args.keep_temps)
    algoddir = os.path.join(tempnet, 'Primary')
    aiport = args.indexer_port or random.randint(4000,30000)
    cmd = [indexer_bin, 'daemon', '-P', psqlstring, '--dev-mode', '--algod', algoddir, '--server', ':{}'.format(aiport)]
    logger.debug("%s", ' '.join(map(repr,cmd)))
    indexerdp = subprocess.Popen(cmd)
    atexit.register(indexerdp.kill)
    time.sleep(0.2)

    indexerurl = 'http://localhost:{}/'.format(aiport)
    healthurl = indexerurl + 'health'
    for attempt in range(20):
        ok = tryhealthurl(healthurl, args.verbose, waitforround=lastblock)
        if ok:
            break
        time.sleep(0.5)
    if not ok:
        logger.error('could not get indexer health')
        return 1
    xrun(['python3', 'misc/validate_accounting.py', '--verbose', '--algod', algoddir, '--indexer', indexerurl], timeout=20)
    xrun(['go', 'run', 'cmd/e2equeries/main.go', '-pg', psqlstring, '-q'], timeout=15)
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
            return False
        raw = response.read()
        logger.debug('health %r', raw)
        ob = json.loads(raw)
        rt = ob.get('message')
        if not rt:
            return False
        return int(rt) >= waitforround
    except Exception as e:
        if verbose:
            logging.warning('GET %s %s', healthurl, e, exc_info=True)
        return False


if __name__ == '__main__':
    sys.exit(main())
