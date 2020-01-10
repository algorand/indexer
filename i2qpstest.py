#!/usr/bin/env python3

import logging
import random
import time

import psycopg2

logger = logging.getLogger(__name__)

# sqlite3 and postgres use different query placeholders:
# sqlite3
# sqlarg = '?'
# psycopg2 postgres:
sqlarg = '%s'

# generator yielding txn msgpack blobs, newest first into the past
def txnbytesForAddr(db, addr):
    with db:
        curs = db.cursor()
        curs.execute('SELECT t.txnbytes FROM txn_participation p JOIN txn t ON t.round = p.round AND t.intra = p.intra WHERE p.addr = {ph} ORDER BY p.addr, P.round DESC, P.intra DESC'.format(ph=sqlarg), (addr,))
        for row in curs:
            txblob = row[0]
            if hasattr(txblob, 'tobytes'):
                txblob = txblob.tobytes()
            yield txblob
        curs.close()

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-f', '--database', default=None, help="psql connect string")
    ap.add_argument('-N', default=10000, type=int, help="number of queries to run")
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)
    
    db = psycopg2.connect(args.database)
    start = time.time()
    with db:
        curs = db.cursor()
        curs.execute('SELECT DISTINCT addr FROM txn_participation')
        addrs = []
        for row in curs:
            addr = row[0]
            if hasattr(addr, 'tobytes'):
                addr = addr.tobytes()
            addrs.append(addr)
        curs.close()
    dt = time.time() - start
    logger.info('loaded %d addrs in %.1fs', len(addrs), dt)
    start = time.time()
    txn_read = 0
    txbytes = 0
    for addr in random.sample(addrs, args.N):
        for txblob in txnbytesForAddr(db, addr):
            txn_read += 1
            txbytes += len(txblob)
    dt = time.time() - start
    logger.info('read %d addrs, %d txns, %d bytes in %.1fs, %.0f qps, %.1f txn/s, %.0f B/s', args.N, txn_read, txbytes, dt, args.N/dt, txn_read/dt, txbytes/dt)
    return

if __name__ == '__main__':
    main()
