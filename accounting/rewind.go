package accounting

import (
	"context"
	"fmt"

	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/protocol"
	models "github.com/algorand/indexer/api/generated/v2"
	itypes "github.com/algorand/indexer/types"

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

var specialAccounts *itypes.SpecialAddresses

// AccountAtRound queries the idb.IndexerDb object for transactions and rewinds most fields of the account back to
// their values at the requested round.
// `round` must be <= `account.Round`
func AccountAtRound(ctx context.Context, account models.Account, round uint64, db idb.IndexerDb) (acct models.Account, err error) {
	// Make sure special accounts cache has been initialized.
	if specialAccounts == nil {
		var accounts itypes.SpecialAddresses
		accounts, err = db.GetSpecialAccounts(ctx)
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
	if specialAccounts.FeeSink == sdk.Address(addr) {
		err = MakeSpecialAccountRewindError("FeeSink")
		return
	}
	if specialAccounts.RewardsPool == sdk.Address(addr) {
		err = MakeSpecialAccountRewindError("RewardsPool")
		return
	}

	// Get transactions and rewind account.
	tf := idb.TransactionFilter{
		Address:  addr[:],
		MinRound: round + 1,
		MaxRound: account.Round,
	}
	ctx2, cf := context.WithCancel(ctx)
	// In case of a panic before the next defer, call cf() here.
	defer cf()
	txns, r := db.Transactions(ctx2, tf)
	// In case of an error, make sure the context is cancelled, and the channel is cleaned up.
	defer func() {
		cf()
		for range txns {
		}
	}()
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
		stxn := txnrow.Txn
		if stxn == nil {
			return models.Account{},
				fmt.Errorf("rewinding past inner transactions is not supported")
		}
		if addr == stxn.Txn.Sender {
			acct.AmountWithoutPendingRewards += stxn.Txn.Fee.ToUint64()
			acct.AmountWithoutPendingRewards -= stxn.SenderRewards.ToUint64()
		}
		switch stxn.Txn.Type {
		case protocol.PaymentTx:
			if addr == stxn.Txn.Sender {
				acct.AmountWithoutPendingRewards += stxn.Txn.Amount.ToUint64()
			}
			if addr == stxn.Txn.Receiver {
				acct.AmountWithoutPendingRewards -= stxn.Txn.Amount.ToUint64()
				acct.AmountWithoutPendingRewards -= stxn.ReceiverRewards.ToUint64()
			}
			if addr == stxn.Txn.CloseRemainderTo {
				// unwind receiving a close-to
				acct.AmountWithoutPendingRewards -= stxn.ClosingAmount.ToUint64()
				acct.AmountWithoutPendingRewards -= stxn.CloseRewards.ToUint64()
			} else if !stxn.Txn.CloseRemainderTo.IsZero() {
				// unwind sending a close-to
				acct.AmountWithoutPendingRewards += stxn.ClosingAmount.ToUint64()
			}
		case protocol.KeyRegistrationTx:
			// TODO: keyreg does not rewind. workaround: query for txns on an account with typeenum=2 to find previous values it was set to.
		case protocol.AssetConfigTx:
			if stxn.Txn.ConfigAsset == 0 {
				// create asset, unwind the application of the value
				assetUpdate(&acct, txnrow.AssetID, 0, stxn.Txn.AssetParams.Total)
			}
		case protocol.AssetTransferTx:
			if addr == stxn.Txn.AssetSender || addr == stxn.Txn.Sender {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), stxn.Txn.AssetAmount+txnrow.Extra.AssetCloseAmount, 0)
			}
			if addr == stxn.Txn.AssetReceiver {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, stxn.Txn.AssetAmount)
			}
			if addr == stxn.Txn.AssetCloseTo {
				assetUpdate(&acct, uint64(stxn.Txn.XferAsset), 0, txnrow.Extra.AssetCloseAmount)
			}
		case protocol.AssetFreezeTx:
		default:
			err = fmt.Errorf("%s[%d,%d]: rewinding past txn type %s is not currently supported", account.Address, txnrow.Round, txnrow.Intra, stxn.Txn.Type)
			return
		}
	}

	acct.Round = round

	// Due to accounts being closed and re-opened, we cannot always rewind Rewards. So clear it out.
	acct.Rewards = 0

	// Computing pending rewards is not supported.
	acct.PendingRewards = 0
	acct.Amount = acct.AmountWithoutPendingRewards

	// TODO: Clear out the closed-at field as well. Like Rewards we cannot know this value for all accounts.
	//acct.ClosedAt = 0

	return
}
