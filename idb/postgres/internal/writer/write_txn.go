package writer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"

	"github.com/algorand/indexer/idb"
	"github.com/algorand/indexer/idb/postgres/internal/encoding"
	"github.com/algorand/indexer/util"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// Get the ID of the creatable referenced in the given transaction
// (0 if not an asset or app transaction).
// Note: ConsensusParams.MaxInnerTransactions could be overridden to force
//       generating ApplyData.{ApplicationID/ConfigAsset}. This function does
//       other things too, so it is not clear we should use it. The only
//       real benefit is that it would slightly simplify this function by
//       allowing us to leave out the intra / block parameters.
func transactionAssetID(stxnad *types.SignedTxnWithAD, intra uint, block *types.Block) (uint64, error) {
	assetid := uint64(0)
	switch stxnad.Txn.Type {
	case types.ApplicationCallTx:
		assetid = uint64(stxnad.Txn.ApplicationID)
		if assetid == 0 {
			assetid = uint64(stxnad.ApplyData.ApplicationID)
		}
		if assetid == 0 {
			if block == nil {
				txid := crypto.TransactionIDString(stxnad.Txn)
				return 0, fmt.Errorf("transactionAssetID(): Missing ApplicationID for transaction: %s", txid)
			}
			// pre v30 transactions do not have ApplyData.ConfigAsset or InnerTxns
			// so txn counter + payset pos calculation is OK
			assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
		}
	case types.AssetConfigTx:
		assetid = uint64(stxnad.Txn.ConfigAsset)
		if assetid == 0 {
			assetid = uint64(stxnad.ApplyData.ConfigAsset)
		}
		if assetid == 0 {
			if block == nil {
				txid := crypto.TransactionIDString(stxnad.Txn)
				return 0, fmt.Errorf("transactionAssetID(): Missing ConfigAsset for transaction: %s", txid)
			}
			// pre v30 transactions do not have ApplyData.ApplicationID or InnerTxns
			// so txn counter + payset pos calculation is OK
			assetid = block.TxnCounter - uint64(len(block.Payset)) + uint64(intra) + 1
		}
	case types.AssetTransferTx:
		assetid = uint64(stxnad.Txn.XferAsset)
	case types.AssetFreezeTx:
		assetid = uint64(stxnad.Txn.FreezeAsset)
	}

	return assetid, nil
}

// Traverses the inner transaction tree and writes database rows
// to `outCh`. It performs a preorder traversal to correctly compute
// the intra round offset, the offset for the next transaction is returned.
func yieldInnerTransactions(ctx context.Context, stxnad *types.SignedTxnWithAD, block *types.Block, intra, rootIntra uint, rootTxid string, outCh chan []interface{}) (uint, error) {
	for _, itxn := range stxnad.ApplyData.EvalDelta.InnerTxns {
		txn := &itxn.Txn
		typeenum, ok := idb.GetTypeEnum(txn.Type)
		if !ok {
			return 0, fmt.Errorf("yieldInnerTransactions() get type enum")
		}
		// block shouldn't be used for inner transactions.
		assetid, err := transactionAssetID(&itxn, 0, nil)
		if err != nil {
			return 0, err
		}
		extra := idb.TxnExtra{
			AssetCloseAmount: itxn.ApplyData.AssetClosingAmount,
			RootIntra:        idb.OptionalUint{Present: true, Value: rootIntra},
			RootTxid:         rootTxid,
		}

		// When encoding an inner transaction we remove any further nested inner transactions.
		// To reconstruct a full object the root transaction must be fetched.
		txnNoInner := itxn
		txnNoInner.EvalDelta.InnerTxns = nil
		row := []interface{}{
			uint64(block.Round), intra, int(typeenum), assetid,
			nil, // inner transactions do not have a txid.
			encoding.EncodeSignedTxnWithAD(txnNoInner),
			encoding.EncodeTxnExtra(&extra)}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("yieldInnerTransactions() ctx.Err(): %w", ctx.Err())
		case outCh <- row:
		}

		// Recurse at end for preorder traversal
		intra, err =
			yieldInnerTransactions(ctx, &itxn, block, intra+1, rootIntra, rootTxid, outCh)
		if err != nil {
			return 0, err
		}
	}

	return intra, nil
}

// Writes database rows for transactions (including inner transactions) to `outCh`.
func yieldTransactions(ctx context.Context, block *types.Block, modifiedTxns []types.SignedTxnInBlock, outCh chan []interface{}) error {
	intra := uint(0)
	for idx, stib := range block.Payset {
		var stxnad types.SignedTxnWithAD
		var err error
		// This function makes sure to set correct genesis information so we can get the
		// correct transaction hash.
		stxnad.SignedTxn, stxnad.ApplyData, err = util.DecodeSignedTxn(block.BlockHeader, stib)
		if err != nil {
			return fmt.Errorf("yieldTransactions() decode signed txn err: %w", err)
		}

		txn := &stxnad.Txn
		typeenum, ok := idb.GetTypeEnum(types.TxType(txn.Type))
		if !ok {
			return fmt.Errorf("yieldTransactions() get type enum")
		}
		assetid, err := transactionAssetID(&stxnad, intra, block)
		if err != nil {
			return err
		}
		id := crypto.TransactionIDString(*txn)

		extra := idb.TxnExtra{
			AssetCloseAmount: modifiedTxns[idx].ApplyData.AssetClosingAmount,
		}
		row := []interface{}{
			uint64(block.Round), intra, int(typeenum), assetid, id,
			encoding.EncodeSignedTxnWithAD(stxnad),
			encoding.EncodeTxnExtra(&extra)}
		select {
		case <-ctx.Done():
			return fmt.Errorf("yieldTransactions() ctx.Err(): %w", ctx.Err())
		case outCh <- row:
		}

		intra, err = yieldInnerTransactions(
			ctx, &stib.SignedTxnWithAD, block, intra+1, intra, id, outCh)
		if err != nil {
			return fmt.Errorf("yieldTransactions() adding inner: %w", err)
		}
	}

	return nil
}

// AddTransactions adds transactions from `block` to the database.
// `modifiedTxns` contains enhanced apply data generated by evaluator.
func AddTransactions(block *types.Block, modifiedTxns []types.SignedTxnInBlock, tx pgx.Tx) error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	ch := make(chan []interface{}, 1024)
	var err0 error
	go func() {
		err0 = yieldTransactions(ctx, block, modifiedTxns, ch)
		close(ch)
	}()

	_, err1 := tx.CopyFrom(
		context.Background(),
		pgx.Identifier{"txn"},
		[]string{"round", "intra", "typeenum", "asset", "txid", "txn", "extra"},
		copyFromChannel(ch))
	if err1 != nil {
		// Exiting here will call `cancelFunc` which will cause the goroutine above to exit.
		return fmt.Errorf("addTransactions() copy from err: %w", err1)
	}

	// CopyFrom() exited successfully, so `ch` has been closed, so `err0` has been
	// written to, and we can read it without worrying about data races.
	if err0 != nil {
		return fmt.Errorf("addTransactions() err: %w", err0)
	}

	return nil
}
