#!/usr/bin/env python3

import json
import logging

import algosdk
import psycopg2
from psycopg2.extras import Json as pgJson

logger = logging.getLogger(__name__)

genesis_json_path = "installer/genesis/mainnet/genesis.json"

# sqlite3
# sqlarg = '?'
# psycopg2 postgres:
sqlarg = '%s'

def get_state(db):
    with db:
        cursor = db.cursor()
        cursor.execute("SELECT v FROM metastate WHERE k = 'state'")
        rows = cursor.fetchall()
        cursor.close()
        if len(rows) == 0:
            return dict()
        if len(rows) == 1:
            return rows[0][0]

def set_state(db, state):
    with db:
        cursor = db.cursor()
        _set_state(cursor, state)
        cursor.close()

def _set_state(cursor, state):
    cursor.execute("INSERT INTO metastate (k, v) VALUES ('state', {ph}) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v".format(ph=sqlarg), (pgJson(state),))


# txnbytes fields:
# included SignedTxn:
# "sig":[32]byte crypto.Signature
# "msig": {MultisigSig}
# "lsig": {LogicSig}
# "txn": {Transaction}
# included ApplyData:
# "ca":uint64 ClosingAmount
# "rs":uint64 SenderRewards
# "rr":uint64 ReceiverRewards
# "rc":uint64 CloseRewards
#
# "hgi":bool HasGenesisID
# "hgh":bool HasGenesisHash
#
# Transaction:
# "type": string TxType
# included Header:
# "snd":[32]byte Sender
# "fee":uint64 Fee
# "fv":uint64 FirstValid
# "lv":uint64 LastValid
# "note":[]byte
# "gen":string GenesisID
# "gh":[32]byte GenesisHash
# included KeyregTxnFields:
# "votekey":[]byte VotePK
# "selkey":[]byte SelectionPK
# "votefst":uint64 VoteFirst
# "votelst":uint64 VoteLast
# "votekd":uint64 VoteKeyDilution
# "nonpart":bool Nonparticipation
# included PaymentTxnFields:
# "rcv":[32]byte Receiver
# "amt":uint64 Amount
# "close":uint64 CloseRemainderTo
# included AssetConfigTxnFields
# "caid":uint64 ConfigAsset (0 on allocate)
# "apar":{AssetParams}
# included AssetTransferTxnFields
# "xaid":uint64 XferAsset
# "aamt":uint64 AssetAmount
# "asnd":[32]byte AssetSender (effective sender for clawback)
# "arcv":[32]byte AssetReceiver
# "aclose":[32]byte AssetCloseTo
# included AssetFreezeTxnFields
# "fadd":[32]byte FreezeAccount
# "faid":uint64 FreezeAsset
# "afrz":uint64 AssetFrozen


# block header fields
# "rnd":uint64 Round
# "prev":[32]byte Branch hash of previous block
# "seed":[]byte Seed
# "txn":[32]byte TxnRoot merkle tree root of txn data
# "ts":int64 TimeStamp unix time
# "gen":string GenesisID
# "gh":[32]byte GenesisHash
# "tc":uint64 TxnCounter
# included RewardsState
# "fees":[32]byte FeeSink
# "rwd":[32]byte RewardsPool
# "earn":uint64 RewardsLevel
# "rate":uint64 RewardsRate
# "frac":uint64 RewardsResidue
# "rwcalr":uint64 RewardsRecalculationRound
# included UpgradeState
# "proto":string CurrentProtocol
# "nextproto":string NextProtocol
# "nextyes":uint64 NextProtocolApprovals
# "nextbefore":uint64 NextProtocolVoteBefore
# "nextswitch":uint64 NextProtocolSwitchOn
# included UpgradeVote
# "upgradeprop":string UpgradePropose
# "upgradedelay":uint64 UpgradeDelay Round
# "upgradeyes":bool UpgradeApprove




def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-g', '--genesis-json', default=genesis_json_path, help="path to your network's genesis.json file")
    ap.add_argument('-f', '--database', default=None, help="psql connect string")
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    db = psycopg2.connect(args.database)

    state = get_state(db)

    account_round = state.get('account_round')
    if account_round is None:
        with open(args.genesis_json, 'rt') as gf:
            genesis = json.load(gf)
        account_rows = []
        for genesis_account in genesis['alloc']:
            addr = algosdk.encoding.decode_address(genesis_account['addr'])
            acctstate = genesis_account['state']
            account_rows.append( (addr, acctstate.get('algo',0), pgJson(acctstate)) )
        with db:
            cursor = db.cursor()
            cursor.executemany("INSERT INTO account (addr, microalgos, account_data) VALUES ({ph}, {ph}, {ph})".format(ph=sqlarg), account_rows)
            state['account_round'] = -1
            _set_state(cursor, state)
            cursor.close()

    with db:
        cursor = db.cursor()
        cursor.execute('SELECT round, intra, txnbytes FROM txn ORDER BY round, intra')
        for row in cursor:
            
    return

if __name__ == '__main__':
    main()
