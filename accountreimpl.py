#!/usr/bin/env python3

import json
import logging
import time

import algosdk
import msgpack
import psycopg2
from psycopg2.extras import Json as pgJson

from indexer2testload import unmsgpack

logger = logging.getLogger(__name__)

genesis_json_path = "installer/genesis/mainnet/genesis.json"

_dev_reset = '''
TRUNCATE account;
TRUNCATE account_asset;
TRUNCATE asset;
TRUNCATE metastate;
'''

# sqlite3
# sqlarg = '?'
# psycopg2 postgres:
sqlarg = '%s'

def sql(query_string_with_placeholder):
    '''workaround sqlite3 and psycopg2 using different arg value placeholders in SQL strings.
{ph} is replaced with ? or %s as needed
'''
    return query_string_with_placeholder.replace('{ph}',sqlarg)

def get_state(db):
    with db:
        cursor = db.cursor()
        result = _get_state(cursor)
        cursor.close()
        return result

def _get_state(cursor):
    cursor.execute("SELECT v FROM metastate WHERE k = 'state'")
    rows = cursor.fetchall()
    if len(rows) == 0:
        return dict()
    if len(rows) == 1:
        return rows[0][0]
    raise Exception("get state expected 1 row but got {}".format(len(rows)))
    

def set_state(db, state):
    with db:
        cursor = db.cursor()
        _set_state(cursor, state)
        cursor.close()

def _set_state(cursor, state):
    cursor.execute(sql("INSERT INTO metastate (k, v) VALUES ('state', {ph}) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v"), (pgJson(state),))

def get_block(db, blockround):
    with db:
        cursor = db.cursor()
        cursor.execute(sql("SELECT header FROM block_header WHERE round = {ph}"), (blockround,))
        rows = cursor.fetchall()
        cursor.close()
        if len(rows) == 1:
            return msgpack.loads(rows[0][0])
        raise Exception("get block {} expected 1 row but got {}".format(blockround, len(rows)))

def get_default_frozen(db):
    # map from asset id to true/false
    default_frozen = {}
    with db:
        cursor = db.cursor()
        cursor.execute(sql("SELECT index, params -> 'df' FROM asset"))
        for row in cursor:
            default_frozen[row[0]] = row[1] or False
    return default_frozen

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
# "grp":[32]byte Group
# "lx":[32]byte Lease
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

# AssetParams
# "t":uint64 Total
# "dc":uint32 Decimals
# "df":bool DefaultFrozen
# "un":string UnitName
# "an":string AssetName
# "au":string URL
# "am":[32]byte MetadataHash
# "m":[32]byte Manager
# "r":[32]byte Reserve
# "f":[32]byte Freeze
# "c":[32]byte Clawback

def load_genesis(db, state, genesis_json):
    with open(genesis_json, 'rt') as gf:
        genesis = json.load(gf)
    account_rows = []
    for genesis_account in genesis['alloc']:
        addr = algosdk.encoding.decode_address(genesis_account['addr'])
        acctstate = genesis_account['state']
        account_rows.append( (addr, acctstate.get('algo',0), pgJson(acctstate)) )
    with db:
        cursor = db.cursor()
        cursor.executemany(sql("INSERT INTO account (addr, microalgos, account_data) VALUES ({ph}, {ph}, {ph})"), account_rows)
        state['account_round'] = -1
        _set_state(cursor, state)
        cursor.close()
    return state

class Accounting:
    def __init__(self, db, state=None):
        self.db = db
        self.current_round = None
        self.block = None
        self._init_round(None)
        self.state = state
        self.default_frozen = get_default_frozen(db)

    def _init_round(self, txround):
        # {addr: algo delta}
        self.algo_updates = {}
        # [(asset id, creator addr, params), ...]
        self.acfg_updates = []
        # {(addr, asset id): asset delta}
        self.asset_updates = {}
        # {(addr, asset id): frozen bool}
        self.freeze_updates = {}
        # [(close to, asset id, sender, sender), ...]
        self.asset_closes = []
        if self.current_round and (txround == self.current_round + 1) and self.block:
            self.txn_counter = self.block.get(b'tc', 0)
        elif txround and txround > 0:
            prevblock = get_block(self.db, txround - 1)
            self.txn_counter = prevblock.get(b'tc', 0)
        self.current_round = txround
        if txround is None:
            self.fee_addr = None
            self.reward_addr = None
            return
        self.block = get_block(self.db, txround)
        self.fee_addr = self.block[b'fees']
        self.reward_addr = self.block[b'rwd']
        return

    def update_from_db(self):
        with self.db:
            cursor = self.db.cursor()
            cursor.execute(sql("SELECT round, intra, txnbytes FROM txn WHERE round > {ph} ORDER BY round, intra"), (account_round,))
            for row in cursor:
                txround, intra, txnbytes = row
                self.add_stxn(txround, intra, txnbytes)
        return

    def _asset_updates_for_db(self):
        for addr_id, delta in self.asset_updates.items():
            addr, assetId = addr_id
            frozen = self.default_frozen[assetId]
            yield (addr, assetId, delta, frozen)

    def _commit_round(self):
        if not self.algo_updates and not self.asset_updates and not self.freeze_updates and not self.asset_closes:
            # nothing happened
            pass
        with self.db:
            cursor = self.db.cursor()
            if self.algo_updates:
                cursor.executemany(sql('''INSERT INTO account (addr, microalgos, account_data) VALUES ({ph}, {ph}, {ph}) ON CONFLICT (addr) DO UPDATE SET microalgos = account.microalgos + EXCLUDED.microalgos, account_data = jsonb_set(account.account_data, '{"algo"}', (account.microalgos + EXCLUDED.microalgos)::text::jsonb, true)'''), map(lambda addr_amt: (addr_amt[0],addr_amt[1],pgJson({'algo':addr_amt[1]})), self.algo_updates.items()))
            if self.acfg_updates:
                cursor.executemany(sql('''INSERT INTO asset (index, creator_addr, params) VALUES ({ph}, {ph}, {ph}) ON CONFLICT (index) DO UPDATE SET params = EXCLUDED.params'''), self.acfg_updates)
            if self.asset_updates:
                # TODO: for INSERT clause merge in default-frozen value
                cursor.executemany(sql('''INSERT INTO account_asset (addr, assetid, amount, frozen) VALUES ({ph}, {ph}, {ph}, {ph}) ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount'''), self._asset_updates_for_db())
            if self.freeze_updates:
                cursor.executemany(sql('''INSERT INTO account_asset (addr, assetid, amount, frozen) VALUES ({ph}, {ph}, 0, {ph}) ON CONFLICT (addr, assetid) DO UPDATE SET frozen = EXCLUDED.frozen'''), map(lambda item: (item[0][0], item[0][1], item[1]), self.freeze_updates.items()))
            if self.asset_closes:
                cursor.executemany(sql('''INSERT INTO account_asset (addr, assetid, amount)
SELECT {ph}, {ph}, x.amount FROM account_asset x WHERE x.addr = {ph}
ON CONFLICT (addr, assetid) DO UPDATE SET amount = account_asset.amount + EXCLUDED.amount;
DELETE FROM account_asset WHERE x.addr = {ph}'''), self.asset_closes)
            if self.state is None:
                self.state = _get_state(cursor)
            self.state['account_round'] = self.current_round
            # TODO: INSERT/UPDATE with psql custom jsonb operators?
            _set_state(cursor, self.state)

    def add_stxn(self, txround, intra, txnbytes):
        if self.current_round is None:
            self._init_round(txround)
        elif self.current_round != txround:
            # commit previous round, start a new one
            self._commit_round()
            self._init_round(txround)
        stxn = msgpack.loads(txnbytes)
        txn = stxn[b'txn']
        ttype = txn[b'type']
        sender = txn[b'snd']
        senderRewards = stxn.get(b'rs')
        fee = txn[b'fee']
        self.algo_updates[sender] = self.algo_updates.get(sender, 0) - fee
        self.algo_updates[self.fee_addr] = self.algo_updates.get(self.fee_addr, 0) + fee
        if senderRewards:
            self.algo_updates[self.reward_addr] = self.algo_updates.get(self.reward_addr, 0) - senderRewards
            self.algo_updates[sender] = self.algo_updates.get(sender, 0) + senderRewards
        if ttype == b'pay':
            amount = txn.get(b'amt')
            receiver = txn.get(b'rcv')
            if amount and receiver:
                self.algo_updates[sender] = self.algo_updates.get(sender, 0) - amount
                self.algo_updates[receiver] = self.algo_updates.get(receiver, 0) + amount
            closeto = txn.get(b'close')
            closeamount = stxn.get(b'ca')
            if closeto and closeamount:
                self.algo_updates[sender] = self.algo_updates.get(sender, 0) - closeamount
                self.algo_updates[closeto] = self.algo_updates.get(closeto, 0) + closeamount
            receiverRewards = stxn.get(b'rr')
            if receiverRewards:
                self.algo_updates[self.reward_addr] = self.algo_updates.get(self.reward_addr, 0) - receiverRewards
                self.algo_updates[receiver] = self.algo_updates.get(receiver, 0) + receiverRewards
            closetoRewards = stxn.get(b'rc')
            if closetoRewards:
                if not closeto:
                    logger.warning('(r=%d,i=%d) rc without close', txround, intra)
                else:
                    self.algo_updates[self.reward_addr] = self.algo_updates.get(self.reward_addr, 0) - closetoRewards
                    self.algo_updates[closeto] = self.algo_updates.get(closeto, 0) + closetoRewards
        elif ttype == b'keyreg':
            pass
        elif ttype == b'acfg':
            assetId = txn.get(b'caid', self.txn_counter + intra + 1)
            params = txn[b'apar']
            self.acfg_updates.append( (assetId, sender, pgJson(unmsgpack(params))) )
            self.default_frozen[assetId] = params.get(b'df', False)
            #logger.error('r=%d block=%r', txround, self.block)
            #raise Exception('(r={},i={}) TODO acfg'.format( txround, intra))
        elif ttype == b'axfer':
            assetId = txn[b'xaid']
            assetSender = txn.get(b'asnd', sender)
            assetReceiver = txn.get(b'arcv')
            assetAmount = txn.get(b'aamt')
            if assetAmount and assetReceiver:
                self.asset_updates[(assetSender,assetId)] = self.asset_updates.get((assetSender,assetId), 0) - assetAmount
                self.asset_updates[(assetReceiver,assetId)] = self.asset_updates.get((assetReceiver,assetId), 0) + assetAmount
            assetCloseTo = txn.get(b'aclose')
            if assetCloseTo:
                self.asset_closes.append( (assetCloseTo, assetId, assetSender, assetSender) )
        elif ttype == b'afrz':
            freezeAccount = txn.get(b'fadd')
            freezeAssetId = txn.get(b'faid')
            isFrozen = txn.get(b'afrz')
            self.freeze_updates[(freezeAccount,freezeAssetId)] = isFrozen
        else:
            raise Exception("unknown txn type {!r} at round={},intra={}".format(ttype, txround, intra))

    def close(self):
        self._commit_round()
        self.db = None
        

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('-g', '--genesis-json', default=genesis_json_path, help="path to your network's genesis.json file")
    ap.add_argument('-f', '--database', default=None, help="psql connect string")
    ap.add_argument('--max-round', default=None, type=int, help='limit progress to < max-round')
    args = ap.parse_args()

    logging.basicConfig(level=logging.INFO)

    db = psycopg2.connect(args.database)

    state = get_state(db)

    lastlog = time.time()
    account_round = state.get('account_round')
    if account_round is None:
        state = load_genesis(db, state, args.genesis_json)
        account_round = state.get('account_round')
    accounting = Accounting(db)
    txcount = 0
    with db:
        cursor = db.cursor()
        cursor.execute(sql("SELECT round, intra, txnbytes FROM txn WHERE round > {ph} ORDER BY round, intra"), (account_round,))
        for row in cursor:
            txround, intra, txnbytes = row
            if args.max_round and txround >= args.max_round:
                break
            accounting.add_stxn(txround, intra, txnbytes)
            txcount += 1
            now = time.time()
            if now - lastlog > 5:
                logger.info('r=%d, i=%d, txns=%d', txround, intra, txcount)
                lastlog = now
    accounting.close()
    return

if __name__ == '__main__':
    main()
