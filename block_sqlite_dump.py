#!/usr/bin/env python3

import json
import logging
import os
import sqlite3
import sys
import time

import msgpack
import algosdk

logger = logging.getLogger(__name__)

def getin(pat, they):
    for x in they:
        if pat in x:
            return x
    return None

def sqliteread(path):
    return sqlite3.connect('file:{}?mode=ro'.format(path), uri=True)

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('dbfile', nargs="+", default=[], help='algod sqlite databases')
    ap.add_argument('-d', '--dir', default=None, help='dir to write block files')
    ap.add_argument('-f', '--force', default=False, action='store_true', help='overwrite block files')
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    blockdir = args.dir or os.getcwd()

    # Look up what round the tracker db is committed through.
    # Later we only export blocks up through that round.
    trackerdbpath = getin('tracker', args.dbfile)
    logger.info('tracker %s', trackerdbpath)
    db = sqliteread(trackerdbpath)
    cursor = db.cursor()
    cursor.execute("select rnd from acctrounds where id = 'acctbase'")
    for row in cursor:
        acctrounds = row[0]
    db.close()

    blockdbpath = getin('block', args.dbfile)
    db = sqliteread(blockdbpath)
    cursor = db.cursor()
    cursor.execute("""SELECT rnd, blkdata, certdata FROM blocks""")
    start = time.time()
    lastlog = start
    count = 0
    for row in cursor:
        rnd, blkdata, certdata = row
        if rnd > acctrounds:
            continue
        blockpath = os.path.join(blockdir, str(rnd))
        if (not args.force) and os.path.exists(blockpath):
            continue
        outob = {
            b'block':msgpack.loads(blkdata),
            b'cert':msgpack.loads(certdata),
        }
        with open(blockpath, 'wb') as fout:
            msgpack.dump(outob, fout)
        count += 1
        now = time.time()
        if (now - lastlog) > 5:
            logger.info('%s', blockpath)
    dt = time.time() - start
    logger.info('%d blocks in %.1f sec', count, dt)
    return

if __name__ == '__main__':
    main()
