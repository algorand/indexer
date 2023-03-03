package writer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"

	"github.com/algorand/go-algorand-sdk/v2/types"
)

// addTransactionParticipants calls function `add` for every address referenced in the
// given transaction, possibly with repetition.
func addTransactionParticipants(stxnad *types.SignedTxnWithAD, includeInner bool, add func(address types.Address)) {
	txn := &stxnad.Txn

	add(txn.Sender)

	switch txn.Type {
	case types.PaymentTx:
		add(txn.Receiver)
		// Close address is optional.
		if !txn.CloseRemainderTo.IsZero() {
			add(txn.CloseRemainderTo)
		}
	case types.AssetTransferTx:
		// If asset sender is non-zero, it is a clawback transaction. Otherwise,
		// the transaction sender address is used.
		if !txn.AssetSender.IsZero() {
			add(txn.AssetSender)
		}
		add(txn.AssetReceiver)
		// Asset close address is optional.
		if !txn.AssetCloseTo.IsZero() {
			add(txn.AssetCloseTo)
		}
	case types.AssetFreezeTx:
		add(txn.FreezeAccount)
	case types.ApplicationCallTx:
		for _, address := range txn.ApplicationCallTxnFields.Accounts {
			add(address)
		}
	}

	if includeInner {
		for _, inner := range stxnad.ApplyData.EvalDelta.InnerTxns {
			addTransactionParticipants(&inner, includeInner, add)
		}
	}
}

// getTransactionParticipants returns referenced addresses from the txn and all inner txns
func getTransactionParticipants(stxnad *types.SignedTxnWithAD, includeInner bool) []types.Address {
	const acctsPerTxn = 7

	if !includeInner || len(stxnad.ApplyData.EvalDelta.InnerTxns) == 0 {
		// if no inner transactions then adding into a slice with in-place de-duplication
		res := make([]types.Address, 0, acctsPerTxn)
		add := func(address types.Address) {
			for _, p := range res {
				if address == p {
					return
				}
			}
			res = append(res, address)
		}

		addTransactionParticipants(stxnad, includeInner, add)
		return res
	}

	// inner transactions might have inner transactions might have inner...
	// so the resultant slice is created after collecting all the data from nested transactions.
	// this is probably a bit slower than the default case due to two mem allocs and additional iterations
	size := acctsPerTxn * (1 + len(stxnad.ApplyData.EvalDelta.InnerTxns)) // approx
	participants := make(map[types.Address]struct{}, size)
	add := func(address types.Address) {
		participants[address] = struct{}{}
	}

	addTransactionParticipants(stxnad, includeInner, add)

	res := make([]types.Address, 0, len(participants))
	for addr := range participants {
		res = append(res, addr)
	}

	return res
}

// addInnerTransactionParticipation traverses the inner transaction tree and
// adds txn participation records for each. It performs a preorder traversal
// to correctly compute the intra round offset, the offset for the next
// transaction is returned.
func addInnerTransactionParticipation(stxnad *types.SignedTxnWithAD, round, intra uint64, rows [][]interface{}) (uint64, [][]interface{}) {
	next := intra
	for _, itxn := range stxnad.ApplyData.EvalDelta.InnerTxns {
		// Only search inner transactions by direct participation.
		// TODO: Should inner app calls be surfaced by their participants?
		participants := getTransactionParticipants(&itxn, false)

		for j := range participants {
			rows = append(rows, []interface{}{participants[j][:], round, next})
		}

		next, rows = addInnerTransactionParticipation(&itxn, round, next+1, rows)
	}
	return next, rows

}

// AddTransactionParticipation writes account participation info to the
// `txn_participation` table.
func AddTransactionParticipation(block *types.Block, tx pgx.Tx) error {
	var rows [][]interface{}
	next := uint64(0)

	for _, stxnib := range block.Payset {
		participants := getTransactionParticipants(&stxnib.SignedTxnWithAD, true)

		for j := range participants {
			rows = append(rows, []interface{}{participants[j][:], uint64(block.Round), next})
		}

		next, rows = addInnerTransactionParticipation(&stxnib.SignedTxnWithAD, uint64(block.Round), next+1, rows)
	}

	_, err := tx.CopyFrom(
		context.Background(),
		pgx.Identifier{"txn_participation"},
		[]string{"addr", "round", "intra"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("addTransactionParticipation() copy from err: %w", err)
	}

	return nil
}
