#!/usr/bin/env python3
#
# pip install py-algorand-sdk

import argparse
import base64
import glob
import json
import logging
import msgpack
import os
import re
import signal
import sys
import tarfile
import time

import algosdk

logger = logging.getLogger(__name__)

# algod = token_addr_from_algod(os.path.join(os.getenv('HOME'),'Algorand/n3/Node1'))
# algod = token_addr_from_algod(os.path.join(os.getenv('HOME'),'mainnet'))
# print(json.dumps(algod.status(), indent=2))
# b=algod.block_info(algod.status()['lastRound'])
# print(json.dumps(b, indent=2))
def token_addr_from_algod(algorand_data):
    addr = open(os.path.join(algorand_data, 'algod.net'), 'rt').read().strip()
    if not addr.startswith('http'):
        addr = 'http://' + addr
    token = open(os.path.join(algorand_data, 'algod.token'), 'rt').read().strip()
    return token, addr

# b = nextblock(algod, b['round'])
def nextblock(algod, lastround=None):
    if lastround is None:
        lastround = algod.status()['lastRound']
        logger.debug('nextblock status lastRound %s', lastround)
    else:
        try:
            b = algod.block_info(lastround + 1)
            return b
        except:
            pass
    status = algod.status_after_block(lastround)
    nbr = status['lastRound']
    b = algod.block_info(nbr)
    return b

def maybedecode(x):
    if isinstance(x, bytes):
        return x.decode()
    return x

def unmsgpack(ob):
    "convert dict from msgpack.loads() with byte string keys to text string keys"
    if isinstance(ob, dict):
        od = {}
        for k,v in ob.items():
            k = maybedecode(k)
            okv = False
            if (not okv) and (k == 'note'):
                try:
                    v = unmsgpack(msgpack.loads(v))
                    okv = True
                except:
                    pass
            if (not okv) and k in ('type', 'note'):
                try:
                    v = v.decode()
                    okv = True
                except:
                    pass
            if not okv:
                v = unmsgpack(v)
            od[k] = v
        return od
    if isinstance(ob, list):
        return [unmsgpack(v) for v in ob]
    #if isinstance(ob, bytes):
    #    return base64.b64encode(ob).decode()
    return ob

def make_ob_json_polite(ob):
    if isinstance(ob, dict):
        return {k:make_ob_json_polite(v) for k,v in ob.items()}
    if isinstance(ob, list):
        return [make_ob_json_polite(x) for x in ob]
    if isinstance(ob, bytes):
        return base64.b64encode(ob).decode()
    return ob

class Algobot:
    def __init__(self, algorand_data=None, token=None, addr=None, headers=None, block_handlers=None, txn_handlers=None, progress_log_path=None, raw_api=None):
        self.algorand_data = algorand_data
        self.token = token
        self.addr = addr
        self.headers = headers
        self._algod = None
        self.block_handlers = block_handlers or list()
        self.txn_handlers = txn_handlers or list()
        self.progress_log_path = progress_log_path
        self._progresslog = None
        self._progresslog_write_count = 0
        self.go = True
        self.raw_api = raw_api
        self.algod_has_block_raw = None
        self.blockfiles = None
        return

    def algod(self):
        if self._algod is None:
            if self.algorand_data:
                token, addr = token_addr_from_algod(self.algorand_data)
            else:
                token = self.token
                addr = self.addr
            self._algod = algosdk.algod.AlgodClient(token, addr, headers=self.headers)
        return self._algod

    def rawblock(self, xround):
        "if possible fetches and returns raw block msgpack including block and cert; otherwise None"
        algod = self.algod()
        if self.algod_has_block_raw or (self.algod_has_block_raw is None):
            response = algod.algod_request("GET", "/block/" + str(xround), params={'raw':1}, raw_response=True)
            contentType = response.getheader('Content-Type')
            if contentType == 'application/json':
                logger.debug('got json response, disabling rawblock')
                self.algod_has_block_raw = False
                return None
            if contentType == 'application/x-algorand-block-v1':
                self.algod_has_block_raw = True
                raw = response.read()
                block = unmsgpack(msgpack.loads(raw))
                return block
            raise Exception('unknown response content type {!r}'.format(contentType))
        logger.debug('rawblock passing out')
        return None

    def eitherblock(self, xround):
        "return raw block or json info block"
        if self.algod_has_block_raw or (self.raw_api != False):
            return self.rawblock(xround)
        if (self.raw_api != False) and (self.algod_has_block_raw is None):
            xb = self.rawblock(xround)
            if self.algod_has_block_raw:
                return xb
        return self.algod().block_info(xround)

    def nextblock_from_files(self):
        if not self.blockfiles:
            logger.debug('empty blockfiles')
            self.go = False
            return {'block':{'rnd':None}}
            #raise Exception("end of blockfiles")
        bf = self.blockfiles[0]
        logger.debug('block from file %s', bf)
        self.blockfiles = self.blockfiles[1:]
        with open(bf, 'rb') as fin:
            raw = fin.read()
        try:
            return unmsgpack(msgpack.loads(raw))
        except Exception as e:
            logger.debug('%s: failed to msgpack decode, %s', bf, e)
        return json.loads(raw.decode())

    def nextblock(self, lastround=None, retries=3):
        "from block_info json api simplified block"
        trycount = 0
        while (trycount < retries) and self.go:
            trycount += 1
            try:
                return self._nextblock_inner(lastround)
            except Exception as e:
                if trycount >= retries:
                    logger.error('too many errors in nextblock retries')
                    raise
                else:
                    logger.warn('error in nextblock(%r) (retrying): %s', lastround, e)
        return None

    def _nextblock_inner(self, lastround):
        if self.blockfiles is not None:
            return self.nextblock_from_files()
        algod = self.algod()
        # TODO: algod block raw
        if lastround is None:
            lastround = algod.status()['lastRound']
            logger.debug('nextblock status lastRound %s', lastround)
        else:
            try:
                return self.eitherblock(lastround + 1)
            except:
                pass
        status = algod.status_after_block(lastround)
        nbr = status['lastRound']
        while (nbr > lastround + 1) and self.go:
            # try lastround+1 one last time
            try:
                return self.eitherblock(lastround + 1)
            except:
                break
        b = self.eitherblock(nbr)
        return b

    def loop(self):
        lastround = self.recover_progress()
        try:
            self._loop_inner(lastround)
        finally:
            self.close()

    def _loop_inner(self, lastround):
        while self.go:
            b = self.nextblock(lastround)
            if b is None:
                print("got None nextblock. exiting")
                return
            nowround = blockround(b)
            if (lastround is not None) and (nowround != lastround + 1):
                logger.info('round jump %d to %d', lastround, nowround)
            for bh in self.block_handlers:
                bh(self, b)
            bb = b.get('block')
            if bb:
                # raw block case
                transactions = bb.get('txns', [])
            else:
                # json block_info case
                txns = b.get('txns', {})
                transactions = txns.get('transactions', [])
            for txn in transactions:
                for th in self.txn_handlers:
                    th(self, b, txn)
            self.record_block_progress(nowround)
            lastround = nowround

    def record_block_progress(self, round_number):
        if self._progresslog_write_count > 100000:
            if self._progresslog is not None:
                self._progresslog.close()
                self._progresslog = None
            nextpath = self.progress_log_path + '_next_' + time.strftime('%Y%m%d_%H%M%S', time.gmtime())
            nextlog = open(nextpath, 'xt')
            nextlog.write('{}\n'.format(round_number))
            nextlog.flush()
            nextlog.close() # could probably leave this open and keep writing to it
            os.replace(nextpath, self.progress_log_path)
            self._progresslog_write_count = 0
            # new log at standard location will be opened next time
            return
        if self._progresslog is None:
            if self.progress_log_path is None:
                return
            self._progresslog = open(self.progress_log_path, 'at')
            self._progresslog_write_count = 0
        self._progresslog.write('{}\n'.format(round_number))
        self._progresslog.flush()
        self._progresslog_write_count += 1

    def recover_progress(self):
        if self.progress_log_path is None:
            return None
        try:
            with open(self.progress_log_path, 'rt') as fin:
                fin.seek(0, 2)
                endpos = fin.tell()
                fin.seek(max(0, endpos - 100))
                raw = fin.read()
                lines = raw.splitlines()
                return int(lines[-1])
        except Exception as e:
            logger.info('could not recover progress: %s', e)
        return None

    def close(self):
        if self._progresslog is not None:
            self._progresslog.close()
            self._progresslog = None

blocktarParseRe = re.compile(r'(\d+)_(\d+).tar.bz2')
            
class BlockArchiver:
    def __init__(self, algorand_data=None, token=None, addr=None, headers=None, blockdir=None, tardir=None):
        self.algorand_data = algorand_data
        self.token = token
        self.addr = addr
        self.headers = headers
        self.blockdir = blockdir
        self.tardir = tardir

        self.storedBlocks = set()
        self.lastBlockOkTime = time.time() # pretend things are okay when we start
        self.go = True
        self._algod = None
        return
    
    def algod(self):
        if self._algod is None:
            if self.algorand_data:
                token, addr = token_addr_from_algod(self.algorand_data)
            else:
                token = self.token
                addr = self.addr
            self._algod = algosdk.algod.AlgodClient(token, addr, headers=self.headers)
        return self._algod

    def lastroundFromBlockdir(self):
        maxround = None
        for fname in os.listdir(self.blockdir):
            try:
                fround = int(fname)
                self.storedBlocks.add(fround)
                if maxround is None or fround > maxround:
                    maxround = fround
            except:
                logger.warning('junk in blockdir: %r', os.path.join(self.blockdir, fname))
        return maxround

    def lastroundFromTardir(self):
        maxround = None
        for fname in os.listdir(self.tardir):
            try:
                m = blocktarParseRe.match(fname)
                if m:
                    endblock = int(m.group(2))
                    if maxround is None or endblock > maxround:
                        maxround = endblock
            except:
                logger.warning('junk in tardir: %r', os.path.join(self.tardir, fname))
        return maxround

    def rawblock(self, xround):
        algod = self.algod()
        logger.debug('get %d', xround)
        response = algod.algod_request("GET", "/block/" + str(xround), params={'raw':1}, raw_response=True)
        contentType = response.getheader('Content-Type')
        if contentType == 'application/x-algorand-block-v1':
            raw = response.read()
            return raw
        return None

    def fetchAndStoreBlock(self, xround):
        raw = self.rawblock(xround)
        if raw is None:
            raise Exception('could not get block {}'.format(xround))
        # trivial check
        bl = msgpack.loads(raw)
        if bl[b'block'][b'rnd'] != xround:
            raise Exception('fetching round {} retrieved block for round {}'.format(xround, bl[b'block'][b'rnd']))
        blockpath = os.path.join(self.blockdir, str(xround))
        with open(blockpath, 'wb') as fout:
            fout.write(raw)
        logger.debug('got block %s', blockpath)
        self.storedBlocks.add(xround)
        self.lastBlockOkTime = time.time()

    def maybeTarBlocks(self):
        minround = min(self.storedBlocks)
        maxround = max(self.storedBlocks)
        xm = minround - (minround % 1000)
        if xm < minround:
            xm += 1000
        if xm+1000 > maxround:
            # not enough blocks
            return
        for r in range(xm, xm+1000):
            if r not in self.storedBlocks:
                self.fetchAndStoreBlock(r)
        # okay, we have them all
        if minround < xm:
            # forget incomplete block set?
            for x in list(self.storedBlocks):
                if x < xm:
                    self.storedBlocks.discard(x)
                    logger.warning('stale block in blockdir: %r', os.path.join(self.blockdir, str(x)))
        tarname = '{}_{}.tar.bz2'.format(xm, xm+1000-1)
        outpath = os.path.join(self.tardir, tarname)
        tf = tarfile.open(outpath, 'w:bz2')
        for r in range(xm, xm+1000):
            bs = str(r)
            tf.add(os.path.join(self.blockdir, bs), arcname=bs)
        tf.close()
        # tar made, cleanup block files
        for r in range(xm, xm+1000):
            bs = str(r)
            os.remove(os.path.join(self.blockdir, bs))
            self.storedBlocks.discard(r)
        # TODO: upload tar to s3
        return

    def _fetchloop(self, lastround):
        while self.go:
            try:
                self.fetchAndStoreBlock(lastround + 1)
                self.maybeTarBlocks()
            except Exception as e:
                logger.warning('err in fetch, %s', e)
                break
            lastround += 1
        return lastround
    
    def run(self):
        lastround = self.lastroundFromBlockdir()
        if lastround is not None:
            logger.debug('lastround from blockdir %d', lastround)
        if lastround is None:
            lastround = self.lastroundFromTardir()
            if lastround is not None:
                logger.debug('lastround from tardir %d', lastround)
        algod = self.algod()
        if lastround is None:
            lastround = algod.status()['lastRound']
            logger.debug('last round from status %d', lastround)
            self.fetchAndStoreBlock(lastround)
        lastround = self._fetchloop(lastround)
        while self.go:
            try:
                algod = self.algod()
                status = algod.status_after_block(lastround)
                logger.debug('status %r', status)
                lastround = self._fetchloop(lastround)
            except Exception as e:
                logger.warning('err in run, %s', e)
                # reset the connection
                self._algod = None
            now = time.time()
            dt = now - self.lastBlockOkTime
            if dt > 30:
                raise Exception('no block for {:.1f}s'.format(dt))
            time.sleep(1)
            

def blockround(b):
    bb = b.get('block')
    if bb:
        # raw mode
        return bb.get('rnd')
    else:
        # block_info json mode
        return b.get('round')

# block_printer is an example block handler; it takes two args, the bot and the block
def block_printer(bot, b):
    txns = b.get('txns')
    if txns:
        print(json.dumps(b, indent=2))
    else:
        bround = b.get('round')
        if bround % 10 == 0:
            print(bround)

# block_counter is an example block handler; it takes two args, the bot and the block
def block_counter(bot, b):
    bround = blockround(b)
    if bround % 10 == 0:
        print(bround)

# big_tx_printer is an example txn handler; it takes three args, the bot the block and the transaction
def big_tx_printer(bot, b, tx):
    txn = tx.get('txn')
    if txn:
        # raw style
        amount = txn.get('amt')
        if amount is not None and amount > 10000000:
            print(json.dumps(make_ob_json_polite(tx), indent=2))
        return
    # block_info style
    payment = tx.get('payment')
    if not payment:
        return
    amount = payment.get('amount')
    if amount > 10000000:
        print(json.dumps(tx, indent=2))

def make_arg_parser():
    ap = argparse.ArgumentParser()
    ap.add_argument('-d', '--algod', default=None, help='algod data dir')
    ap.add_argument('-a', '--addr', default=None, help='algod host:port address')
    ap.add_argument('-t', '--token', default=None, help='algod API access token')
    ap.add_argument('--header', dest='headers', nargs='*', help='"Name: value" HTTP header (repeatable)')
    ap.add_argument('--verbose', default=False, action='store_true')
    #ap.add_argument('--progress-file', default=None, help='file to write progress to')
    #ap.add_argument('--blockfile-glob', default=None, help='file glob of block files')
    #ap.add_argument('--raw-api', default=False, action='store_true', help='use raw msgpack api with more data but different layout than json block_info api')
    ap.add_argument('--blockdir')
    ap.add_argument('--tardir')
    return ap

def header_list_to_dict(hlist):
    if not hlist:
        return None
    p = re.compile(r':\s+')
    out = {}
    for x in hlist:
        a, b = p.split(x, 1)
        out[a] = b
    return out

def setup(args):
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    algorand_data = args.algod or os.getenv('ALGORAND_DATA')
    if not algorand_data and not (args.token and args.addr):
        sys.stderr.write('must specify algod data dir by $ALGORAND_DATA or -d/--algod; OR --a/--addr and -t/--token\n')
        sys.exit(1)

    bot = BlockArchiver(
        algorand_data,
        token=args.token,
        addr=args.addr,
        headers=header_list_to_dict(args.headers),
        blockdir=args.blockdir,
        tardir=args.tardir,
    )

    #if args.blockfile_glob:
    #    bot.blockfiles = glob.glob(args.blockfile_glob)

    killcount = [0]
    def gogently(signum, stackframe):
        count = killcount[0] + 1
        if count == 1:
            sys.stderr.write('signal received. starting graceful shutdown\n')
            bot.go = False
            killcount[0] = count
            return
        sys.stderr.write('second signal received. bye\n')
        sys.exit(1)

    signal.signal(signal.SIGTERM, gogently)
    signal.signal(signal.SIGINT, gogently)
    return bot

def main(arghook=None):
    ap = make_arg_parser()
    args = ap.parse_args()

    if arghook is not None:
        arghook(args)

    bot = setup(args)
    bot.run()
    return

if __name__ == '__main__':
    main()
