#!/usr/bin/env python3

import os
import tarfile

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--delete', default=False, action='store_true', help="delete files that have been archived to tar files")
    ap.add_argument('-o', '--outdir', default='.', help='dir to store tar files')
    ap.add_argument('-n', '--blocks-per-file', type=int, default=1000, help='number of blocks to put in each tar archive. note that a full block is about 1 MB')
    ap.add_argument('-i', '--indir', default='.', help='dir to list for block files')
    args = ap.parse_args()
    
    haveblocks = os.listdir(args.indir)
    blocknums = sorted(map(int, haveblocks))
    # check continuous sequence
    prev = None
    stop = None
    for x in blocknums:
        if prev is not None and (prev != (x - 1)):
            print("bad sequence: prev = {}, cur = {}".format(prev, x))
            stop = Prev
        prev = x

    batch = []
    for x in blocknums:
        if x == stop:
            break
        batch.append(x)
        if len(batch) >= args.blocks_per_file:
            fname = '{}_{}.tar.bz2'.format(batch[0], batch[-1])
            path = os.path.join(args.outdir, fname)
            print(path)
            tout = tarfile.open(path, 'w:bz2')
            for bi in batch:
                bi = str(bi)
                tout.add(os.path.join(args.indir, bi), arcname=bi)
            tout.close()
            if args.delete:
                for bi in batch:
                    os.remove(os.path.join(args.indir, str(bi)))
            batch = []

if __name__ == '__main__':
    main()
