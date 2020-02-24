#!/usr/bin/env python3

import base64
import json
import sys
import urllib.request

import algosdk

encode_addr = algosdk.encoding.encode_address

def niceaddrfmt(addr):
    if len(addr) == 32:
        return encode_addr(addr)
    try:
        addr = base64.b64decode(addr)
        return encode_addr(addr)
    except:
        return addr
        

def format_stxnw(stxnw):
    stxn = stxnw['stxn']
    txn = stxn['txn']
    sender = txn.pop('snd')
    parts = ['{}:{}'.format(stxnw['r'], stxnw['o']), 's={}'.format(niceaddrfmt(sender))]
    asnd = txn.pop('asnd', None)
    if asnd:
        parts.append('as={}'.format(niceaddrfmt(asnd)))
    receiver = txn.pop('rcv', None)
    if receiver:
        parts.append('r={}'.format(niceaddrfmt(receiver)))
    closeto = txn.pop('close', None)
    if closeto:
        parts.append('c={}'.format(niceaddrfmt(closeto)))
    arcv = txn.pop('arcv', None)
    if arcv:
        parts.append('ar={}'.format(niceaddrfmt(arcv)))
    aclose = txn.pop('aclose', None)
    if aclose:
        parts.append('ac={}'.format(niceaddrfmt(aclose)))
    # everything else
    parts.append(json.dumps(txn))
    return '\t'.join(parts)

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--server', default='localhost:8080', help='host:port of indexer')
    ap.add_argument('-a', '--addr', dest='addritem', default=None)
    ap.add_argument('addr', nargs='?', default=None)
    args = ap.parse_args()
    addr = args.addr
    if args.addr and args.addritem:
        sys.stderr.write('Should only have one of positional addr arg or -a/--addr\n')
        sys.exit(1)
    addr = args.addr or args.addritem
    url = 'http://{}/v1/account/{}/transactions?format=json'.format(args.server, addr)
    response = urllib.request.urlopen(url)
    ob = json.loads(response.read())
    txns = ob.get('txns')
    if not txns:
        return
    for stxnw in txns:
        sys.stdout.write(format_stxnw(stxnw) + '\n')
        pass
    return

if __name__ == '__main__':
    main()
