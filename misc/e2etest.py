#!/usr/bin/env python3

import atexit
import boto3
import glob
import logging
import os
import random
import subprocess
import sys
import tempfile
import time

from botocore.config import Config
from botocore import UNSIGNED

logger = logging.getLogger(__name__)

defaultTimeout = 30 # seconds

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


def maybe_decode(x):
    if hasattr(x, 'decode'):
        return x.decode()
    return x

def _getio(p, od, ed):
    if od is not None:
        od = maybe_decode(od)
    elif p.stdout:
        try:
            od = maybe_decode(p.stdout.read())
        except:
            logger.error('subcomand out', exc_info=True)
    if ed is not None:
        ed = maybe_decode(ed)
    elif p.stderr:
        try:
            ed = maybe_decode(p.stderr.read())
        except:
            logger.error('subcomand err', exc_info=True)
    return od, ed

def xrun(cmd, *args, **kwargs):
    timeout = kwargs.pop('timeout', None)
    kwargs['stdout'] = subprocess.PIPE
    kwargs['stderr'] = subprocess.STDOUT
    try:
        p = subprocess.Popen(cmd, *args, **kwargs)
    except Exception as e:
        logger.error('subprocess failed {!r}'.format(cmd), exc_info=True)
        raise
    stdout_data, stderr_data = None, None
    try:
        if timeout:
            stdout_data, stderr_data = p.communicate(timeout=timeout)
        else:
            stdout_data, stderr_data = p.communicate()
    except subprocess.TimeoutExpired as te:
        cmdr = repr(cmd)
        logger.error('subprocess timed out {}'.format(cmdr), exc_info=True)
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write('output from {}:\n{}\n\n'.format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write('stderr from {}:\n{}\n\n'.format(cmdr, stderr_data))
        raise
    except Exception as e:
        cmdr = repr(cmd)
        logger.error('subprocess exception {}'.format(cmdr), exc_info=True)
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write('output from {}:\n{}\n\n'.format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write('stderr from {}:\n{}\n\n'.format(cmdr, stderr_data))
        raise
    if p.returncode != 0:
        cmdr = repr(cmd)
        logger.error('cmd failed ({}) {}'.format(p.returncode, cmdr))
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write('output from {}:\n{}\n\n'.format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write('stderr from {}:\n{}\n\n'.format(cmdr, stderr_data))
        raise Exception('error: cmd failed: {}'.format(cmdr))

def atexitrun(cmd, *args, **kwargs):
    cargs = [cmd]+list(args)
    atexit.register(xrun, *cargs, **kwargs)

def main():
    dbname = 'e2eindex_{}_{}'.format(int(time.time()), random.randrange(1000))

    if len(sys.argv) == 1:
        # No arguments, assume local postgres instance
        xrun(['dropdb', '--if-exists', dbname], timeout=5)
        xrun(['createdb', dbname], timeout=5)
        atexitrun(['dropdb', '--if-exists', dbname], timeout=5)
        psqlstring = 'dbname={} sslmode=disable'.format(dbname)

    else:
        # Otherwise assume a connection string was provided
        psqlstring = sys.argv[1]

    start = time.time()
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--keep-temps', default=False, action='store_true')
    ap.add_argument('--verbose', default=False, action='store_true')
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    e2edata = os.getenv('E2EDATA')
    if not e2edata:
        tdir = tempfile.TemporaryDirectory()
        e2edata = tdir.name
        if not args.keep_temps:
            atexit.register(tdir.cleanup)
        else:
            logger.info("leaving temp dir %r", tdir.name)
    ensureTestData(e2edata)

    dbname = 'e2eindex_{}_{}'.format(int(time.time()), random.randrange(1000))
    xrun(['dropdb', '--if-exists', dbname], timeout=5)
    xrun(['createdb', dbname], timeout=5)
    if not args.keep_temps:
        atexitrun(['dropdb', '--if-exists', dbname], timeout=5)
    else:
        logger.info("leaving db %r", dbname)
    psqlstring = 'dbname={} sslmode=disable'.format(dbname)
    xrun(['cmd/algorand-indexer/algorand-indexer', 'import', '-P', psqlstring, os.path.join(e2edata, 'blocktars', '*'), '--genesis', os.path.join(e2edata, 'algod', 'genesis.json')], timeout=20)
    cmd = ['cmd/algorand-indexer/algorand-indexer', 'daemon', '-P', psqlstring, '--dev-mode', '--no-algod']
    logger.debug("%s", ' '.join(map(repr,cmd)))
    indexerdp = subprocess.Popen(cmd)
    atexit.register(indexerdp.kill)
    time.sleep(0.2)
    sqliteglob = os.path.join(e2edata, 'algod', '*', 'ledger.tracker.sqlite')
    sqlitepaths = glob.glob(sqliteglob)
    sqlitepath = sqlitepaths[0]
    xrun(['python3', 'misc/validate_accounting.py', '--verbose', '--dbfile', sqlitepath, '--indexer', 'http://localhost:8980/'], timeout=20)
    dt = time.time() - start
    sys.stdout.write("indexer e2etest OK ({:.1f}s)\n".format(dt))
    return 0

if __name__ == '__main__':
    sys.exit(main())
