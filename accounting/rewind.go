package accounting

import (
	"context"
	"fmt"

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/protocol"
	models "github.com/algorand/indexer/api/generated/v2"

	"github.com/algorand/indexer/idb"
)

// ConsistencyError is returned when the database returns inconsistent (stale) results.
type ConsistencyError struct {
	msg string
}

func (e ConsistencyError) Error() string {
	return e.msg
}

func assetUpdate(account *models.Account, assetid uint64, add, sub uint64) {
	if account.Assets == nil {
		account.Assets = new([]models.AssetHolding)
	}
	assets := *account.Assets
	for i, ah := range assets {
		if ah.AssetId == assetid {
			ah.Amount += add
			ah.Amount -= sub
			assets[i] = ah
			// found and updated asset, done
			return
		}
	}
	// add asset to list
	assets = append(assets, models.AssetHolding{
		Amount:  add - sub,
		AssetId: assetid,
		//Creator: base32 addr string of asset creator, TODO
		//IsFrozen: leave nil? // TODO: on close record frozen state for rewind
	})
	*account.Assets = assets
}

// SpecialAccountRewindError indicates that an attempt was made to rewind one of the special accounts.
type SpecialAccountRewindError struct {
	account string
}

// MakeSpecialAccountRewindError helper to initialize a SpecialAccountRewindError.
func MakeSpecialAccountRewindError(account string) *SpecialAccountRewindError {
	return &SpecialAccountRewindError{account: account}
}

// Error is part of the error interface.
func (sare *SpecialAccountRewindError) Error() string {
	return fmt.Sprintf("unable to rewind the %s", sare.account)
}

var specialAccounts *transactions.SpecialAddresses

// AccountAtRound queries the idb.IndexerDb object for transactions and rewinds most fields of the account back to
// their values at the requested round.
// `round` must be <= `account.Round`
func AccountAtRound(account models.Account, round uint64, db idb.IndexerDb) (acct models.Account, err error) {
	// Make sure special accounts cache has been initialized.
	if specialAccounts == nil {
		var accounts transactions.SpecialAddresses
		accounts, err = db.GetSpecialAccounts()
		if err != nil {
			return models.Account{}, fmt.Errorf("unable to get special accounts: %v", err)
		}
		specialAccounts = &accounts
	}

	acct = account
	var addr basics.Address
	addr, err = basics.UnmarshalChecksumAddress(account.Address)
	if err != nil {
		return
	}

	// ensure that the don't attempt to rewind a special account.
	if basics.Address(specialAccounts.FeeSink) == addr {
		err = MakeSpecialAccountRewindError("FeeSink")
		return
	}
	if basics.Address(specialAccounts.RewardsPool) == addr {
		err = MakeSpecialAccountRewindError("RewardsPool")
		return
	}

	// Get transactions and rewind account.
	tf := idb.TransactionFilter{
		Address:  addr[:],
		MinRound: round + 1,
		MaxRound: account.Round,
	}
	txns, r := db.Transactions(context.Background(), tf)
	if r < account.Round {
		err = ConsistencyError{fmt.Sprintf("queried round r: %d < account.Round: %d", r, account.Round)}
		return
	}
	txcount := 0
	for txnrow := range txns {
		if txnrow.Error != nil {
			err = txnrow.Error
			return
		}
		txcount++
		var stxn transactions.SignedTxnWithAD
		err = protocol.Decode(txnrow.TxnBytes, &stxn)
		if err != nil {
			return
		}
		if addr == basics.Address(stxn.Txn.Sender) {
			acct.AmountWithoutPendingRewards += stxn.Txn.Fee.Raw
			acct.AmountWithoutPendingRewards -= stxn.SenderRewards.Raw
			acct.Rewards -= stxn.SenderRewards.Raw
		}
		switch protocol.TxType(stxn.Txn.Type) {
		case protocol.PaymentTx:
			if addr == basics.Address(stxn.Txn.Sender) {
				acct.AmountWithoutPendingRewards += stxn.Txn.Amount.Raw
			}
			if addr == basics.Address(stxn.Txn.Receiver) {
				acct.AmountWithoutPendingRewards -= stxn.Txn.Amount.Raw
				acct.AmountWithoutPendingRewards -= stxn.ReceiverRewards.Raw
				acct.Rewards -= stxn.ReceiverRewards.Raw
			}
			if addr == basics.Address(stxn.Txn.CloseRemainderTo) {
				// unwind receiving a close-to
				acct.AmountWithoutPendingRewards -= stxn.ClosingAmount.Raw
				acct.AmountWithoutPendingRewards -= stxn.CloseRewards.Raw
				acct.Rewards -= stxn.CloseRewards.Raw
			} else if !stxn.Txn.CloseRemainderTo.IsZero() {
				// unwind sending a close-to
				acct.AmountWithoutPendingRewards += stxn.ClosingAmount.Raw
			}
		case protocol.KeyRegistrationTx:
			// TODO: keyreg does not rewind. workaround: query for txns on an account with typeenum=2 to find previous values it was set to.
		case protocol.AssetConfigTx:
			if stxn.Txn.ConfigAsset == 0 {
				// create asset, unwind the application of the value
				assetUpdate(&acct, txnrow.AssetID, 0, stxn.Txn.AssetParams.Total)
			}
		case protocol.AssetTransferTx:
			if addr == basics.Address(stxn.Txn.AssetSender) || addr == basics.Address(stxn.Txn.Sender) {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetAmount+txnrow.Extra.AssetCloseAmount, 0)
			}
			if addr == basics.Address(stxn.Txn.AssetReceiver) {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, stxn.Txn.AssetAmount)
			}
			if addr == basics.Address(stxn.Txn.AssetCloseTo) {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, txnrow.Extra.AssetCloseAmount)
			}
		case protocol.AssetFreezeTx:
		default:
			err = fmt.Errorf("%s[%d,%d]: rewinding past txn type %s is not currently supported", account.Address, txnrow.Round, txnrow.Intra, stxn.Txn.Type)
			return
		}
	}

	if txcount > 0 {
		// If we found any txns above, we need to find one
		// more so we can know what the previous RewardsBase
		// of the account was so we can get the accurate
		// pending rewards at the target round.
		//
		// (If there weren't any txns above, the recorded
		// RewardsBase is current from whatever previous txn
		// happened to this account.)

		tf.MaxRound = round
		tf.MinRound = 0
		tf.Limit = 1
		txns, r = db.Transactions(context.Background(), tf)
		if r < round {
			err = ConsistencyError{
				fmt.Sprintf("queried round r: %d < requested round round: %d", r, round)}
			return
		}
		for txnrow := range txns {
			if txnrow.Error != nil {
				err = txnrow.Error
				return
			}
			var stxn transactions.SignedTxnWithAD
			err = protocol.Decode(txnrow.TxnBytes, &stxn)
			if err != nil {
				return
			}
			var baseBlockHeader bookkeeping.BlockHeader
			baseBlockHeader, _, err = db.GetBlock(context.Background(), txnrow.Round, idb.GetBlockOptions{})
			if err != nil {
				return
			}
			prevRewardsBase := baseBlockHeader.RewardsLevel
			var blockheader bookkeeping.BlockHeader
			blockheader, _, err = db.GetBlock(context.Background(), round, idb.GetBlockOptions{})
			if err != nil {
				return
			}
			proto, ok := config.Consensus[blockheader.CurrentProtocol]
			if !ok {
				return
			}
			rewardsUnits := acct.AmountWithoutPendingRewards / proto.RewardUnit
			rewardsDelta := blockheader.RewardsLevel - prevRewardsBase
			acct.PendingRewards = rewardsDelta * rewardsUnits
			acct.Amount = acct.PendingRewards + acct.AmountWithoutPendingRewards
			acct.Round = round
			return
		}

		// There were no prior transactions, it must have been empty before, zero out things
		acct.PendingRewards = 0
		acct.Amount = acct.AmountWithoutPendingRewards
	}

	acct.Round = round

	// Due to accounts being closed and re-opened, we cannot always rewind Rewards. So clear it out.
	acct.Rewards = 0

	// TODO: Clear out the closed-at field as well. Like Rewards we cannot know this value for all accounts.
	//acct.ClosedAt = 0

	return
}
