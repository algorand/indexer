#!/usr/bin/env python3

import atexit
import logging
import os
import random
import subprocess
import sys
import time

logger = logging.getLogger(__name__)


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
    if logger.isEnabledFor(logging.DEBUG):
        logger.debug('cmd success: %r\n%s\n%s\n', cmd, maybe_decode(stdout_data), maybe_decode(stderr_data))

def atexitrun(cmd, *args, **kwargs):
    cargs = [cmd]+list(args)
    atexit.register(xrun, *cargs, **kwargs)

def find_indexer(indexer_bin, exc=True):
    if indexer_bin:
        return indexer_bin
    # manually search local build and PATH for algorand-indexer
    path = ['cmd/algorand-indexer'] + os.getenv('PATH').split(':')
    for pd in path:
        ib = os.path.join(pd, 'algorand-indexer')
        if os.path.exists(ib):
            return ib
    msg = 'could not find algorand-indexer. use --indexer-bin or PATH environment variable.'
    if exc:
        raise Exception(msg)
    logger.error(msg)
    return None

def ensure_test_db(connection_string, keep_temps=False):
    if connection_string:
        # use the passed db
        return connection_string
    # create a temporary database
    dbname = 'e2eindex_{}_{}'.format(int(time.time()), random.randrange(1000))
    xrun(['dropdb', '--if-exists', dbname], timeout=5)
    xrun(['createdb', dbname], timeout=5)
    if not keep_temps:
        atexitrun(['dropdb', '--if-exists', dbname], timeout=5)
    else:
        logger.info("leaving db %r", dbname)
    return 'dbname={} sslmode=disable'.format(dbname)
