#!/usr/bin/env python3
#
# pip install requests

import json
import os
import subprocess
import time

# dig srv _algobootstrap._tcp.mainnet.algorand.network
# curl -o 1 http://relay-montreal-mainnet-algorand.algorand-mainnet.network.:4160/v1/mainnet-v1.0/block/1


cmap = [chr(x) for x in range(ord('0'), ord('9')+1)] + [chr(x) for x in range(ord('a'), ord('z')+1)]

def intToBase36(x):
    if x is None:
        return ''
    if x == 0:
        return '0'
    out = ''
    while x > 0:
        remainder = x % 36
        out = cmap[remainder] + out
        x = x // 36
    return out

def blockUrl(block_number, host, port=4160, genesis_id='mainnet-v1.0'):
    return 'http://{}:{}/v1/{}/block/{}'.format(host, port, genesis_id, intToBase36(block_number))

def fetchLoop(host, port=4160, genesis_id=None, block_number=None, exit_on_err=True):
    import requests
    session = requests.session()
    lastlog = time.time()
    haveblocks = set(os.listdir())
    if block_number is None:
        block_number = min(map(int, haveblocks))
    prev_block_file_path = None
    while True:
        block_file_path = '{:d}'.format(block_number)
        if block_file_path in haveblocks:
            #if os.path.exists(block_file_path):
            block_number += 1
            continue
        url = blockUrl(block_number, host, port, genesis_id)
        response = session.get(url)
        if not response.ok:
            print('GET {} {}'.format(url, response.status_code))
            break
        with open(block_file_path, 'wb') as fout:
            fout.write(response.content)
        prev_block_file_path = block_file_path
        now = time.time()
        dt = now - lastlog
        if dt > 5.0:
            print(block_file_path)
            lastlog = now
        block_number += 1
    if prev_block_file_path:
        print(prev_block_file_path)
    return

def run_from_algod(algorand_data):
    genesisfile = os.path.join(algorand_data, 'genesis.json')
    with open(genesisfile, rt) as fin:
        ob = json.load(fin)
    genesis_id = ob['network'] + '-' + ob['id']
    try:
        result = subprocess.run(['dig', '_algobootstrap._tcp.{}.algorand.network'.format(ob['network']), 'SRV'], stdout=subprocess.PIPE, timeout=20)
    except:
        pass
    # TODO: parse response lines like
    # _algobootstrap._tcp.mainnet.algorand.network. 150 IN SRV 1 1 4160 r-si.algorand-mainnet.network.
    return

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-d', '--algod', default=None, help='algod data dir')
    ap.add_argument('--host', default='relay-montreal-mainnet-algorand.algorand-mainnet.network.', help='archival host to fetch from')
    ap.add_argument('--genesis-id', default='mainnet-v1.0')
    args = ap.parse_args()

    if args.host:
        # host='relay-montreal-mainnet-algorand.algorand-mainnet.network.'
        if not args.genesis_id:
            sys.stderr.write('--genesis-id is required with --host\n')
            sys.exit(1)
            return
        fetchLoop(host=args.host, genesis_id=args.genesis_id)
        return
    if args.algod:
        run_from_algod(args.algod)
        return

if __name__ == '__main__':
    main()
