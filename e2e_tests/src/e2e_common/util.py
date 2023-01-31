#!/usr/bin/env python3

import atexit
import logging
import os
import random
import sqlite3
import subprocess
import sys
import time

import msgpack

logger = logging.getLogger(__name__)


def maybedecode(x):
    if hasattr(x, "decode"):
        return x.decode()
    return x


def mloads(x):
    return msgpack.loads(x, strict_map_key=False, raw=True)


def unmsgpack(ob):
    "convert dict from msgpack.loads() with byte string keys to text string keys"
    if isinstance(ob, dict):
        od = {}
        for k, v in ob.items():
            k = maybedecode(k)
            okv = False
            if (not okv) and (k == "note"):
                try:
                    v = unmsgpack(mloads(v))
                    okv = True
                except:
                    pass
            if (not okv) and k in ("type", "note"):
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
    # if isinstance(ob, bytes):
    #    return base64.b64encode(ob).decode()
    return ob


def _getio(p, od, ed):
    if od is not None:
        od = maybedecode(od)
    elif p.stdout:
        try:
            od = maybedecode(p.stdout.read())
        except:
            logger.error("subcomand out", exc_info=True)
    if ed is not None:
        ed = maybedecode(ed)
    elif p.stderr:
        try:
            ed = maybedecode(p.stderr.read())
        except:
            logger.error("subcomand err", exc_info=True)
    return od, ed


def xrun(cmd, *args, **kwargs):
    timeout = kwargs.pop("timeout", None)
    kwargs["stdout"] = subprocess.PIPE
    kwargs["stderr"] = subprocess.STDOUT
    cmdr = " ".join(map(repr, cmd))
    try:
        p = subprocess.Popen(cmd, *args, **kwargs)
    except Exception as e:
        logger.error("subprocess failed {}".format(cmdr), exc_info=True)
        raise
    stdout_data, stderr_data = None, None
    try:
        if timeout:
            stdout_data, stderr_data = p.communicate(timeout=timeout)
        else:
            stdout_data, stderr_data = p.communicate()
    except subprocess.TimeoutExpired as te:
        logger.error("subprocess timed out {}".format(cmdr), exc_info=True)
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write("output from {}:\n{}\n\n".format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write("stderr from {}:\n{}\n\n".format(cmdr, stderr_data))
        raise
    except Exception as e:
        logger.error("subprocess exception {}".format(cmdr), exc_info=True)
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write("output from {}:\n{}\n\n".format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write("stderr from {}:\n{}\n\n".format(cmdr, stderr_data))
        raise
    if p.returncode != 0:
        logger.error("cmd failed ({}) {}".format(p.returncode, cmdr))
        stdout_data, stderr_data = _getio(p, stdout_data, stderr_data)
        if stdout_data:
            sys.stderr.write("output from {}:\n{}\n\n".format(cmdr, stdout_data))
        if stderr_data:
            sys.stderr.write("stderr from {}:\n{}\n\n".format(cmdr, stderr_data))
        raise Exception("error: cmd failed: {}".format(cmdr))
    if logger.isEnabledFor(logging.DEBUG):
        logger.debug(
            "cmd success: %s\n%s\n%s\n",
            cmdr,
            maybedecode(stdout_data),
            maybedecode(stderr_data),
        )


def atexitrun(cmd, *args, **kwargs):
    cargs = [cmd] + list(args)
    atexit.register(xrun, *cargs, **kwargs)


def find_binary(binary, exc=True, binary_name="algorand-indexer"):
    if binary:
        return binary
    # manually search local build and PATH for binary_name
    path = [f"cmd/{binary_name}"] + os.getenv("PATH").split(":")
    for pd in path:
        ib = os.path.join(pd, binary_name)
        if os.path.exists(ib):
            return ib
    msg = f"could not find {binary_name} at the provided location or PATH environment variable."
    if exc:
        raise Exception(msg)
    logger.error(msg)
    return None


def ensure_test_db(connection_string, keep_temps=False):
    if connection_string:
        # use the passed db
        return connection_string
    # create a temporary database
    dbname = "e2eindex_{}_{}".format(int(time.time()), random.randrange(1000))
    xrun(["dropdb", "--if-exists", dbname], timeout=5)
    xrun(["createdb", dbname], timeout=5)
    if not keep_temps:
        atexitrun(["dropdb", "--if-exists", dbname], timeout=5)
    else:
        logger.info("leaving db %r", dbname)
    return "dbname={} sslmode=disable".format(dbname)


# whoever calls this will need to import boto and get the s3 client
def firstFromS3Prefix(
    s3, bucket, prefix, desired_filename, outdir=None, outpath=None
) -> bool:
    haystack = []
    found_needle = False

    response = s3.list_objects_v2(Bucket=bucket, Prefix=prefix, MaxKeys=100)
    if (not response.get("KeyCount")) or ("Contents" not in response):
        raise Exception("nothing found in s3://{}/{}".format(bucket, prefix))
    for x in response["Contents"]:
        path = x["Key"]
        haystack.append(path)
        _, fname = path.rsplit("/", 1)
        if fname == desired_filename:
            if outpath is None:
                if outdir is None:
                    outdir = "."
                outpath = os.path.join(outdir, desired_filename)
            logger.info("s3://%s/%s -> %s", bucket, x["Key"], outpath)
            s3.download_file(bucket, x["Key"], outpath)
            found_needle = True
            break

    if not found_needle:
        logger.warning("file {} not found in s3://{}/{}".format(desired_filename, bucket, prefix))
    return found_needle


def countblocks(path):
    db = sqlite3.connect(path)
    cursor = db.cursor()
    cursor.execute("SELECT max(rnd) FROM blocks")
    row = cursor.fetchone()
    cursor.close()
    db.close()
    return row[0]


def hassuffix(x, *suffixes):
    for s in suffixes:
        if x.endswith(s):
            return True
    return False
