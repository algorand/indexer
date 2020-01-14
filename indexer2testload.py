#!/usr/bin/env python3

import base64
import datetime
import glob
import json
import logging
import msgpack
import os
#import sqlite3
import tarfile
import time

import psycopg2
from psycopg2.extras import Json as pgJson

logger = logging.getLogger(__name__)

def maybedecode(x):
    if isinstance(x, bytes):
        return x.decode()
    return x

def unmsgpack(ob):
    if isinstance(ob, dict):
        od = {}
        for k,v in ob.items():
            k = maybedecode(k)
            okv = False
            #if k in ('type', 'note'):
            if k == 'type':
                try:
                    v = v.decode()
                    okv = True
                except:
                    pass
            # if (not okv) and (k == 'note'):
            #     try:
            #         v = unmsgpack(msgpack.loads(v))
            #         okv = True
            #     except:
            #         pass
            if not okv:
                v = unmsgpack(v)
            od[k] = v
        return od
    if isinstance(ob, list):
        return [unmsgpack(v) for v in ob]
    if isinstance(ob, bytes):
        return base64.b64encode(ob).decode()
    return ob

# maintain a mapping from some symbol to int.
# new symbols are assigned the next int value.
# e.g. map 32 byte account string to int account id
def hlookup(d, k):
    if k is None:
        return None
    if k in d:
        return d[k]
    v = len(d)
    d[k] = v
    return v

#create_block_table = '''CREATE TABLE IF NOT EXISTS block (round bigint PRIMARY KEY, timestamp bigint)'''
#create_payment_table = '''CREATE TABLE IF NOT EXISTS payment (round bigint, intra smallint, sender_id bigint, receiver_id bigint, closeto_id bigint, amount bigint, fee bigint, PRIMARY KEY (round, intra))'''
#create_account_table = '''CREATE TABLE IF NOT EXISTS account (addr bytea PRIMARY KEY, id bigint)'''
#create_import_table = '''CREATE TABLE IF NOT EXISTS imported (path text)'''
#create_lsigs_table = '''CREATE TABLE IF NOT EXISTS lsigs (round bigint, intra smallint, sender_id bigint, lsig bytea, args bytea, PRIMARY KEY (round, intra))'''


# sqlite3
# sqlarg = '?'
# psycopg2 postgres:
sqlarg = '%s'

# byte strings because msgpack decodes txn bytes that way
typeenumMap = {
    b'pay':1,
    b'keyreg':2,
    b'acfg':3,
    b'axfer':4,
    b'afrz':5,
}
typeenumNames = {
    b'pay':'Payment',
    b'keyreg':'KeyRegistration',
    b'acfg':'AssetConfig',
    b'axfer':'AssetTransfer',
    b'afrz':'AssetFreeze',
}

create_typeemap_table = '''CREATE TABLE IF NOT EXISTS txntypes (typeenum smallint PRIMARY KEY, shortname varchar(32), name varchar(32))'''

def setup_txntypes(db):
    with db:
        curs = db.cursor()
        curs.execute(create_typeemap_table)
        for shortname, typeenum in typeenumMap.items():
            name = typeenumNames[shortname]
            shortname = shortname.decode()
            curs.execute('INSERT INTO txntypes (typeenum, shortname, name) VALUES ({ph}, {ph}, {ph}) ON CONFLICT DO NOTHING'.format(ph=sqlarg), (typeenum, shortname, name))
        curs.close()

class Processor:
    def __init__(self, args, db):
        self.count = 0
        self.txcount = 0
        #self.lastlog = time.time()
        self.args = args
        self.db = db
        self.prior_imports = set()
        self.prior_accounts = {}
        self.accounts = {}
        self.amount_total = 0

    def setup(self):
        db = self.db
        setup_path = os.path.join(os.path.dirname(__file__), 'setup_indexer2.sql')
        with open(setup_path, 'rt') as fin:
            setup_script = fin.read()
        with db:
            curs = db.cursor()
            curs.execute(setup_script)
            curs.close()
        with db:
            curs = db.cursor()
            curs.execute("SELECT path FROM imported")
            self.prior_imports = set([row[0] for row in curs.fetchall()])
            # curs.execute("SELECT addr, id FROM account")
            # for row in curs:
            #     addr = row[0]
            #     if hasattr(addr, 'tobytes'):
            #         addr = addr.tobytes()
            #     self.prior_accounts[addr] = row[1]
            curs.close()
        #print("{} prior accounts, {} prior imports".format(len(self.prior_accounts), len(self.prior_imports)))
        #self.accounts = dict(self.prior_accounts)

    def insertBlock(self, db, block):
        blockround = block[b'block'].get(b'rnd', 0)
        blocktimestamp = block[b'block'][b'ts']
        #if count < 5:
        #    print(json.dumps(unmsgpack(block), indent=2))
        curs = db.cursor()
        intra = 0
        for stx in block[b'block'].get(b'txns',[]):
            self.txcount += 1
            #if txcount < 100:
            #    print(json.dumps(unmsgpack(stx), indent=2))
            tx = stx[b'txn']
            sender = tx.get(b'snd')
            #sid = hlookup(self.accounts, sender)
            txtype = tx[b'type']
            txtypeenum = typeenumMap[txtype]
            receiver = tx.get(b'rcv')
            closeto = tx.get(b'close')
            clawedbackfrom = tx.get(b'asnd')
            assetreceiver = tx.get(b'arcv')
            assetcloseto = tx.get(b'aclose')
            fee = tx[b'fee']
            amt = tx.get(b'amt')
            assetid = 0
            if txtype == b'acfg':
                assetid = tx.get(b'caid', 0)
            elif txtype == b'axfer':
                assetid = tx.get(b'xaid', 0)
            elif txtype == b'afrz':
                assetid = tx.get(b'faid', 0)
            if amt is not None:
                self.amount_total += amt
            participants = filter(None, set([sender, receiver, closeto, clawedbackfrom, assetreceiver, assetcloseto]))
            #rid = hlookup(self.accounts, receiver)
            #cid = hlookup(self.accounts, closeto)
            # TODO: use algosdk algo-canonical msgpack encoding
            try:
                curs.execute('INSERT INTO txn (round, intra, typeenum, asset, txnbytes, txn) VALUES ({ph}, {ph}, {ph}, {ph}, {ph}, {ph}) ON CONFLICT DO NOTHING'.format(ph=sqlarg), (blockround, intra, txtypeenum, assetid, msgpack.dumps(stx), pgJson(unmsgpack(stx))))
            except:
                logger.error("stx raw=%r", stx)
                logger.error("stx unm=%r", unmsgpack(stx))
                raise
            for addr in participants:
                curs.execute('INSERT INTO txn_participation (addr, round, intra) VALUES ({ph}, {ph}, {ph}) ON CONFLICT DO NOTHING'.format(ph=sqlarg), (addr, blockround, intra))
            
            intra += 1
        blockheader = dict(block[b'block'])
        blockheader.pop(b'txns', None)
        # TODO: use algosdk algo-canonical msgpack encoding
        curs.execute("INSERT INTO block_header (round, realtime, header) VALUES ({ph}, {ph}, {ph}) ON CONFLICT DO NOTHING".format(ph=sqlarg), (blockround, datetime.datetime.fromtimestamp(blocktimestamp, datetime.timezone.utc), msgpack.dumps(blockheader)))
        curs.close()
        db.commit()

    def processTarfile(self, tfname):
        db = self.db
        tf = tarfile.open(tfname)
        imported_all = False
        while True:
            tfi = tf.next()
            if tfi is None:
                imported_all = True
                break
            #print('{} {}'.format(tfname, tfi.name))
            fin = tf.extractfile(tfi)
            block = msgpack.load(fin)
            self.insertBlock(db, block)
            self.count += 1
            if self.args.blocklimit > 0 and self.count > self.args.blocklimit:
                imported_all = False
                break
        tf.close()
        return imported_all

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-d', '--blocktars', default=None, help="directory where blocktars are to be loaded from")
    ap.add_argument('-f', '--database', default=None, help="psql connect string")
    ap.add_argument('--blocklimit', type=int, default=0)
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    start = time.time()
    lastlog = start
    lastblockcount = 0
    lasttxcount = 0
    #prior_accounts = {} # map addr to int
    #prior_imports = set()
    db = psycopg2.connect(args.database)
    setup_txntypes(db)
    proc = Processor(args, db)
    proc.setup()
    pattern = os.path.join(args.blocktars, '*_*.tar.bz2')
    imported = []
    for tfname in glob.glob(pattern):
        if tfname in proc.prior_imports:
            continue
        print(tfname)
        imported_all = proc.processTarfile(tfname)
        if not imported_all:
            break
        with db:
            curs = db.cursor()
            curs.execute('INSERT INTO imported (path) VALUES ({ph})'.format(ph=sqlarg), (tfname,))
            for addr, i in proc.accounts.items():
                if addr not in proc.prior_accounts:
                    curs.execute('INSERT INTO account (addr, id) VALUES ({ph}, {ph})'.format(ph=sqlarg), (addr, i))
            proc.prior_accounts = dict(proc.accounts)
            curs.close()
        now = time.time()
        tdt = now - start
        ldt = now - lastlog
        if ldt > 5:
            dblocks = proc.count - lastblockcount
            dtx = proc.txcount - lasttxcount
            print('{:9d} blocks, {:9d} txns, {:6} accounts, {:9.2f} algos; in {:.1f} seconds ({:.1f} block/s, {:.2f} txn/s)'.format(proc.count, proc.txcount, len(proc.accounts), proc.amount_total/1000000.0, tdt, dblocks/ldt, dtx/ldt))
            lastlog = now
            lastblockcount = proc.count
            lasttxcount = proc.txcount
        if args.blocklimit > 0 and proc.count > args.blocklimit:
            break
    dt = time.time() - start
    print('{:9d} blocks, {:9d} txns, {:6} accounts, {:9.2f} algos; in {:.1f} seconds ({:.1f} block/s, {:.2f} txn/s)'.format(proc.count, proc.txcount, len(proc.accounts), proc.amount_total/1000000.0, dt, proc.count/dt, proc.txcount/dt))
            
    return

if __name__ == '__main__':
    main()
