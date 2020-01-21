#!/usr/bin/env python3
#
# Dump algod tracker database to show all current algo and asset balances.
# Prints json per line {"addr":"", "v":{"algo":0, "123":0}, "f":{"123":true}}
#
# usage:
#  python tracker_balance_dump.py -f mainnet-v1.0/ledger.tracker.sqlite

import json
import logging
import sqlite3
import sys

import msgpack
import algosdk

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
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    out = sys.stdout

    db = sqlite3.connect(args.dbfile)
    cursor = db.cursor()
    cursor.execute('''SELECT address, data FROM accountbase''')
    count = 0
    for row in cursor:
        address, data = row
        adata = msgpack.loads(data)
        count += 1
        values = {"algo": adata[b'algo']}
        assets = adata.get(b'asset')
        frozen = {}
        if assets:
            for assetidstr, assetdata in assets.items():
                if isinstance(assetidstr, bytes):
                    assetidstr = assetidstr.decode()
                elif isinstance(assetidstr, str):
                    pass
                else:
                    assetidstr = str(assetidstr)
                values[assetidstr] = assetdata[b'a']
                if assetdata.get(b'f'):
                    frozen[assetidstr] = True
        ob = {
            "addr": algosdk.encoding.encode_address(address),
            "v": values,
        }
        if frozen:
            ob['f'] = frozen
        out.write(json.dumps(ob) + '\n')
        if args.limit and count > args.limit:
            break
    return


if __name__ == '__main__':
    main()

