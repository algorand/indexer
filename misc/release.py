#!/usr/bin/env python3
#
# usage:
# python3 -m venv ve3
# ve3/bin/pip install markdown2
# ve3/bin/python misc/release.py

import argparse
import base64
import io
import logging
import os
import re
import shlex
import subprocess
import tarfile
import time

try:
    import grp

    def groupname(gid):
        return grp.getgrgid(gid)[0]

except:

    def groupname(gid):
        return str(gid)


import markdown2

logger = logging.getLogger(__name__)

# GOOS GOARCH DEB_HOST_ARCH
osArchArch = [
    ("linux", "amd64", "amd64"),
    ("linux", "arm", "armhf"),
    ("linux", "arm64", "arm64"),
    ("darwin", "amd64", None),
    ("darwin", "arm64", None)
]

channel = "indexer"

indexer_filespec = [
    # [files], source path, deb path, tar path
    [
        ["algorand-indexer.service", "algorand-indexer@.service"],
        "misc/systemd",
        "lib/systemd/system",
        "",
    ],
    [
        ["algorand-indexer"],
        "cmd/algorand-indexer",
        "usr/bin",
        "",
    ],
    [
        ["LICENSE"],
        "",
        None,
        "",
    ],
]

conduit_filespec = [
    [
        ["conduit"],
        "cmd/conduit",
        "usr/bin",
        ""
    ],
    [
        ["LICENSE"],
        "",
        None,
        "",
    ],
]

debian_copyright_top = """Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: Algorand Indexer
Upstream-Contact: Algorand developers <dev@algorand.com>
Source: https://github.com/algorand/indexer

Files: *
Copyright: Algorand developers <dev@algorand.com>
License: MIT
"""


def debian_copyright(outpath):
    with open(outpath, "wt") as fout:
        fout.write(debian_copyright_top)
        with open("LICENSE") as fin:
            for line in fin:
                line = line.strip()
                if not line:
                    line = " .\n"
                else:
                    line = " " + line + "\n"
                fout.write(line)


def arch_ver(outpath, inpath, debarch, version):
    with open(outpath, "wt") as fout:
        with open(inpath) as fin:
            for line in fin:
                line = line.replace("@ARCH@", debarch)
                line = line.replace("@VER@", version)
                line = line.replace("@CHANNEL@", channel)
                fout.write(line)


def link(sourcepath, destpath):
    if os.path.exists(destpath):
        if os.path.getmtime(destpath) >= os.path.getmtime(sourcepath):
            return  # nothing to do
        os.remove(destpath)
    os.link(sourcepath, destpath)


_tagpat = re.compile(r"tag:\s+([^,\n]+)")


def compile_version_opts(release_version=None, allow_mismatch=False):
    result = subprocess.run(
        ["git", "log", "-n", "1", "--pretty=%H %D"], stdout=subprocess.PIPE
    )
    result.check_returncode()
    so = result.stdout.decode()
    githash, desc = so.split(None, 1)
    tags = []
    tag = None
    for m in _tagpat.finditer(desc):
        tag = m.group(1)
        tags.append(tag)
        if tag == release_version:
            break
    if tag != release_version:
        if not allow_mismatch:
            raise Exception(
                ".version is {!r} but tags {!r}".format(release_version, tags)
            )
        else:
            logger.warning(".version is %r but tags %r", release_version, tags)
    now = time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime()) + "+0000"
    result = subprocess.run(["git", "status", "--porcelain"], stdout=subprocess.PIPE)
    result.check_returncode()
    if len(result.stdout) > 2:
        dirty = "true"
    else:
        dirty = ""
    # Note: keep these in sync with Makefile
    ldflags = "-ldflags=-X github.com/algorand/indexer/version.Hash={}".format(githash)
    ldflags += " -X github.com/algorand/indexer/version.Dirty={}".format(dirty)
    ldflags += " -X github.com/algorand/indexer/version.CompileTime={}".format(now)
    ldflags += " -X github.com/algorand/indexer/version.GitDecorateBase64={}".format(
        base64.b64encode(desc.encode()).decode()
    )
    if release_version:
        ldflags += " -X github.com/algorand/indexer/version.ReleaseVersion={}".format(
            release_version
        )
    logger.debug(
        "Hash=%r Dirty=%r CompileTime=%r decorate=%r ReleaseVersion=%r",
        githash,
        dirty,
        now,
        desc,
        release_version,
    )
    logger.debug("%s", ldflags)
    return ldflags


def compile(path="cmd/algorand-indexer", goos=None, goarch=None, ldflags=None):
    env = dict(os.environ)
    if goos is not None:
        env["GOOS"] = goos
    if goarch is not None:
        env["GOARCH"] = goarch
    cmd = ["go", "build"]
    if ldflags is not None:
        cmd.append(ldflags)
    subprocess.run(["go", "generate", "./..."], env=env).check_returncode()
    subprocess.run(cmd, cwd=path, env=env).check_returncode()


def build_deb(debarch, version, filespec, outdir):
    os.makedirs(".deb_tmp/DEBIAN", exist_ok=True)
    debian_copyright(".deb_tmp/DEBIAN/copyright")
    arch_ver(".deb_tmp/DEBIAN/control", "misc/debian/control", debarch, version)
    os.makedirs(".deb_tmp/etc/apt/apt.conf.d", exist_ok=True)
    arch_ver(
        ".deb_tmp/etc/apt/apt.conf.d/52algorand-indexer-upgrades",
        "misc/debian/52algorand-indexer-upgrades",
        debarch,
        version,
    )
    for files, source_path, deb_path, _ in filespec:
        if deb_path is None:
            continue
        for fname in files:
            if deb_path:
                os.makedirs(os.path.join(".deb_tmp", deb_path), exist_ok=True)
            link(
                os.path.join(source_path, fname),
                os.path.join(".deb_tmp", deb_path, fname),
            )
    debname = "algorand-indexer_{}_{}.deb".format(version, debarch)
    debpath = os.path.join(outdir, debname)
    subprocess.run(["dpkg-deb", "--build", ".deb_tmp", debpath])
    return debpath


def extract_usage():
    usage = False
    usageBuffer = ""
    with open("README.md") as infile:
        for line in infile:
            if "USAGE_START_MARKER" in line:
                usage = True
                continue
            elif "USAGE_END_MARKER" in line:
                usage = False
                continue
            elif usage:
                usageBuffer += line
    return usageBuffer


_usage_html = None


def usage_html():
    global _usage_html
    if _usage_html is not None:
        return _usage_html
    md = extract_usage()
    _usage_html = markdown2.markdown(md, extras=["tables", "fenced-code-blocks"])
    return _usage_html


def build_tar(name, goos, goarch, version, filespec, outdir):
    rootdir = "{}_{}_{}_{}".format(name, goos, goarch, version)
    tarname = os.path.join(outdir, rootdir) + ".tar.bz2"
    tf = tarfile.open(tarname, "w:bz2")
    for files, source_path, _, tar_path in filespec:
        if tar_path is None:
            continue
        for fname in files:
            tf.add(
                os.path.join(source_path, fname), os.path.join(rootdir, tar_path, fname)
            )
    ti = tarfile.TarInfo(name=os.path.join(rootdir, "usage.html"))
    ti.mtime = time.time()
    ti.mode = 0o444
    ti.type = tarfile.REGTYPE
    ti.uid = os.getuid()
    ti.uname = os.getenv("USER") or ""
    ti.gid = os.getgid()
    ti.gname = groupname(os.getgid())
    uhtml = usage_html().encode("utf-8")
    ti.size = len(uhtml)
    tf.addfile(ti, io.BytesIO(uhtml))
    tf.close()
    return tarname


def hostOsArch():
    result = subprocess.run(["go", "env"], stdout=subprocess.PIPE)
    result.check_returncode()
    goenv = {}
    for line in result.stdout.decode().splitlines():
        line = line.strip()
        k, v = line.split("=", 1)
        goenv[k] = shlex.split(v)[0]
    return goenv["GOHOSTOS"], goenv["GOHOSTARCH"]


def main():
    start = time.time()
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "-o",
        "--outdir",
        help="The output directory for the build assets",
        type=str,
        default=".",
    )
    ap.add_argument(
        "--no-deb",
        action="store_true",
        default=False,
        help="disable debian package building",
    )
    ap.add_argument(
        "--host-only",
        action="store_true",
        default=False,
        help="only build for host OS and CPU",
    )
    ap.add_argument(
        "--build-only",
        action="store_true",
        default=False,
        help="don't make tar or deb release",
    )
    ap.add_argument(
        "--fake-release",
        action="store_true",
        default=False,
        help="relax some checks during release script development",
    )
    ap.add_argument("--verbose", action="store_true", default=False)
    args = ap.parse_args()

    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    outdir = args.outdir

    if args.host_only:
        hostos, hostarch = hostOsArch()
        logger.info("will only run %s %s", hostos, hostarch)
    with open(".version") as fin:
        version = fin.read().strip()
    ldflags = compile_version_opts(version, allow_mismatch=args.fake_release)
    for goos, goarch, debarch in osArchArch:
        if args.host_only and (goos != hostos or goarch != hostarch):
            logger.debug("skip %s %s", goos, goarch)
            continue
        logger.info("GOOS=%s GOARCH=%s DEB_HOST_ARCH=%s", goos, goarch, debarch)
        compile("cmd/algorand-indexer", goos, goarch, ldflags)
        compile("cmd/conduit", goos, goarch)
        if args.build_only:
            logger.debug("skip packaging")
            continue
        indexer_tarname = build_tar("algorand-indexer", goos, goarch, version, indexer_filespec, outdir)
        logger.info("\t%s", indexer_tarname)
        conduit_tarname = build_tar("conduit", goos, goarch, version, conduit_filespec, outdir)
        logger.info("\t%s", conduit_tarname)
        if (not args.no_deb) and (debarch is not None):
            debname = build_deb(debarch, version, indexer_filespec, outdir)
            logger.info("\t%s", debname)
    dt = time.time() - start
    logger.info("done %0.1fs", dt)
    return


if __name__ == "__main__":
    main()
