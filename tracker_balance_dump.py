#!/usr/bin/env python3
#
# Dump algod tracker database to show all current algo and asset balances.
# Prints json per line {"addr":"", "v":{"algo":0, "123":0}, "f":{"123":true}}
#
# usage:
#  python tracker_balance_dump.py -f mainnet-v1.0/ledger.tracker.sqlite
#
# setup requires:
#  pip install msgpack py-algorand-sdk

import json
import logging
import sqlite3
import sys

import msgpack
import algosdk
import psycopg2

from indexer2testload import unmsgpack

logger = logging.getLogger(__name__)

# data/basics/userBalance.go AccountData
# "onl":uint Status
# "algo":uint64 MicroAlgos

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-f', '--dbfile', default=None, help='sqlite3 tracker file from algod')
    ap.add_argument('--limit', default=None, type=int, help='debug limit number of accoutns to dump')
    ap.add_argument('--i2db', default=None, help='psql connect string for indexer 2 db')
    ap.add_argument('--asset', default=None, help='filter on accounts possessing asset id')
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    out = sys.stdout
    err = sys.stderr

    i2db = {}
    
    if args.i2db:
        pg = psycopg2.connect(args.i2db)
        cursor = pg.cursor()
        cursor.execute("SELECT addr, microalgos, rewardsbase FROM account")
        for row in cursor:
            addr, microalgos, rewardsbase = row
            addr = addr.tobytes()
            i2db[addr] = (microalgos, rewardsbase)
        cursor.close()
        pg.close()

    db = sqlite3.connect(args.dbfile)
    cursor = db.cursor()
    cursor.execute('''SELECT address, data FROM accountbase''')
    count = 0
    match = 0
    neq = 0
    for row in cursor:
        address, data = row
        niceaddr = algosdk.encoding.encode_address(address)
        adata = msgpack.loads(data)
        count += 1
        rewardsbase = adata.get(b'ebase', 0)
        microalgos = adata[b'algo']
        values = {"algo": microalgos}
        assets = adata.get(b'asset')
        frozen = {}
        has_asset = False
        if assets:
            for assetidstr, assetdata in assets.items():
                if isinstance(assetidstr, bytes):
                    assetidstr = assetidstr.decode()
                elif isinstance(assetidstr, str):
                    pass
                else:
                    assetidstr = str(assetidstr)
                if assetidstr == args.asset:
                    has_asset = True
                values[assetidstr] = assetdata[b'a']
                if assetdata.get(b'f'):
                    frozen[assetidstr] = True
        if args.asset and not has_asset:
            continue
        if args.i2db:
            i2v = i2db.pop(address, None)
            if i2v is None:
                neq += 1
                err.write('{} not in i2db\n'.format(niceaddr))
            else:
                if i2v[0] == microalgos:
                    match += 1
                else:
                    err.write('{} algod v={} i2db v={}\n'.format(niceaddr, microalgos, i2v[0]))
                    neq += 1
        ob = {
            "addr": niceaddr,
            "v": values,
            "ebase": rewardsbase,
        }
        if frozen:
            ob['f'] = frozen
        out.write(json.dumps(ob) + '\n')
        if args.limit and count > args.limit:
            break
    if args.i2db:
        for addr, i2v in i2db.items():
            if i2v[0] == 0:
                # we keep records on 0 balance accounts but algod doesn't. okay. ignore that.
                continue
            err.write('{} in i2db but not algod\n'.format(addr))
        err.write('{} match {} neq\n'.format(match, neq))
    return


if __name__ == '__main__':
    main()

