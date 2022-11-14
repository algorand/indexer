package writer

import (
	"context"
	"fmt"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/accounting"
)

// getTransactionParticipants returns referenced addresses from the txn and all inner txns
func getTransactionParticipants(stxnad *transactions.SignedTxnWithAD, includeInner bool) []types.Address {
	const acctsPerTxn = 7

	if !includeInner || len(stxnad.ApplyData.EvalDelta.InnerTxns) == 0 {
		// if no inner transactions then adding into a slice with in-place de-duplication
		res := make([]types.Address, 0, acctsPerTxn)
		add := func(address basics.Address) {
			for _, p := range res {
				if types.Address(address) == p {
					return
				}
			}
			res = append(res, types.Address(address))
		}

		accounting.GetTransactionParticipants(stxnad, includeInner, add)
		return res
	}

	// inner transactions might have inner transactions might have inner...
	// so the resultant slice is created after collecting all the data from nested transactions.
	// this is probably a bit slower than the default case due to two mem allocs and additional iterations
	size := acctsPerTxn * (1 + len(stxnad.ApplyData.EvalDelta.InnerTxns)) // approx
	participants := make(map[types.Address]struct{}, size)
	add := func(address basics.Address) {
		participants[types.Address(address)] = struct{}{}
	}

	accounting.GetTransactionParticipants(stxnad, includeInner, add)

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
func addInnerTransactionParticipation(stxnad *transactions.SignedTxnWithAD, round, intra uint64, rows [][]interface{}) (uint64, [][]interface{}) {
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
func AddTransactionParticipation(block *bookkeeping.Block, tx pgx.Tx) error {
	var rows [][]interface{}
	next := uint64(0)

	for _, stxnib := range block.Payset {
		participants := getTransactionParticipants(&stxnib.SignedTxnWithAD, true)

		for j := range participants {
			rows = append(rows, []interface{}{participants[j][:], uint64(block.Round()), next})
		}

		next, rows = addInnerTransactionParticipation(&stxnib.SignedTxnWithAD, uint64(block.Round()), next+1, rows)
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
