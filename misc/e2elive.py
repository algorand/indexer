#!/usr/bin/env python3
#

import atexit
import json
import logging
import os
import random
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
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    indexer_bin = find_indexer(args.indexer_bin)
    e2edata = os.getenv('E2EDATA')
    sourcenet = os.path.join(e2edata, 'net')
    if not os.path.isdir(sourcenet):
        logger.error('need network at %r', sourcenet)
        return 1
    tdir = tempfile.TemporaryDirectory()
    tempnet = os.path.join(tdir.name, 'net')
    if not args.keep_temps:
        atexit.register(tdir.cleanup)
    else:
        logger.info("leaving temp dir %r", tdir.name)
    xrun(['rsync', '-a', sourcenet + '/', tempnet + '/'])
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
        ok = tryhealthurl(healthurl, args.verbose)
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

def tryhealthurl(healthurl, verbose=False):
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
        return int(rt) > 100
    except Exception as e:
        if verbose:
            logging.warning('GET %s %s', healthurl, e, exc_info=True)
        return False


if __name__ == '__main__':
    sys.exit(main())
