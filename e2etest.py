#!/usr/bin/env python3

import atexit
import logging
import os
import random
import subprocess
import sys
import time

logger = logging.getLogger(__name__)

defaultTimeout = 30 # seconds

def _getio(p, od, ed):
    if od is None and p.stdout:
        try:
            od = p.stdout.read()
        except:
            logger.error('subcomand out', exc_info=True)
    if ed is None and p.stderr:
        try:
            ed = p.stderr.read()
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
    start = time.time()
    logging.basicConfig(level=logging.INFO)
    e2edata = os.getenv('E2EDATA') or os.path.join(os.getenv('HOME'), 'Algorand', 'e2edata')

    dbname = 'e2eindex_{}_{}'.format(int(time.time()), random.randrange(1000))
    xrun(['dropdb', '--if-exists', dbname], timeout=5)
    xrun(['createdb', dbname], timeout=5)
    atexitrun(['dropdb', '--if-exists', dbname], timeout=5)
    psqlstring = 'dbname={} sslmode=disable'.format(dbname)
    xrun(['cmd/indexer/indexer', 'import', '-P', psqlstring, os.path.join(e2edata, 'blocktars', '*'), '--genesis', os.path.join(e2edata, 'algod', 'genesis.json')], timeout=20)
    cmd = ['cmd/indexer/indexer', 'daemon', '-P', psqlstring, '--dev-mode', '--no-algod']
    indexerdp = subprocess.Popen(cmd)
    atexit.register(indexerdp.kill)
    time.sleep(0.2)
    xrun(['python3', 'validate_accounting.py', '--dbfile', os.path.join(e2edata, 'algod', 'tbd-v1', 'ledger.tracker.sqlite'), '--indexer', 'http://localhost:8980'], timeout=20)
    dt = time.time() - start
    sys.stdout.write("indexer e2etest OK ({:.1f}s)\n".format(dt))
    return 0

if __name__ == '__main__':
    sys.exit(main())
