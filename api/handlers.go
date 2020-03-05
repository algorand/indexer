package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/algorand/go-algorand-sdk/client/algod/models"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	sdk_types "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/indexer/accounting"
	"github.com/algorand/indexer/importer"
	"github.com/algorand/indexer/types"
	"net/http"
	"strings"
	"time"

	"github.com/algorand/indexer/api/generated"
	"github.com/algorand/indexer/idb"
	"github.com/labstack/echo/v4"
)

type ServerImplementation struct {}

func badRequest(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusBadRequest, generated.Error{
		Error: err,
	})
}

func uintOrDefault(x *uint64, def uint64) uint64 {
	if x != nil {
		return *x
	}
	return def
}
func uint64Ptr(x uint64) *uint64 {
	return &x
}

func strPtr(x string) *string {
	if len(x) == 0 {
		return nil
	}
	return &x
}

func boolPtr(x bool) *bool {
	return &x
}

type genesis struct {
	genesisHash []byte
	genesisID string
}

func getGenesis(ctx context.Context) genesis {
	// TODO: Use 'fetchBlock' helper to lookup these values
	return genesis {
		genesisHash: []byte("TODO"),
		genesisID:   "TODO",
	}
}

func assetHoldingToAssetHolding(id uint64, holding models.AssetHolding) generated.AssetHolding {
	return generated.AssetHolding{
		AssetId:  id,
		Amount:   holding.Amount,
		Creator:  holding.Creator,
		IsFrozen: boolPtr(holding.Frozen),
	}
}

func assetParamsToAsset(id uint64, params models.AssetParams) generated.Asset {
	return generated.Asset{
		Index: id,
		Params: generated.AssetParams{
			Clawback:      strPtr(params.ClawbackAddr),
			Creator:       params.Creator,
			Decimals:      uint64(params.Decimals),
			DefaultFrozen: boolPtr(params.DefaultFrozen),
			Freeze:        strPtr(params.FreezeAddr),
			Manager:       strPtr(params.ManagerAddr),
			MetadataHash:  strPtr(string(params.MetadataHash)),
			Name:          strPtr(params.AssetName),
			Reserve:       strPtr(params.ReserveAddr),
			Total:         params.Total,
			UnitName:      strPtr(params.UnitName),
			Url:           strPtr(params.URL),
		},
	}
}

func accountToAccount(account models.Account) generated.Account {
	// TODO: This data is missing.
	var participation = generated.AccountParticipation{
			SelectionParticipationKey: strPtr(""),
			VoteFirstValid:            uint64Ptr(0),
			VoteLastValid:             uint64Ptr(0),
			VoteKeyDilution:           uint64Ptr(0),
			VoteParticipationKey:      strPtr(""),
	}

	var assets = make([]generated.AssetHolding, 0)
	for k, v := range account.Assets {
		assets = append(assets, assetHoldingToAssetHolding(k, v))
	}

	var createdAssets = make([]generated.Asset, 0)
	for k, v := range account.AssetParams {
		createdAssets = append(createdAssets, assetParamsToAsset(k, v))
	}

	ret := generated.Account{
		Address:                     account.Address,
		Amount:                      account.Amount,
		AmountWithoutPendingRewards: account.AmountWithoutPendingRewards,
		Assets:                      &assets,
		CreatedAssets:               &createdAssets,
		Participation:               &participation,
		PendingRewards:              account.PendingRewards,
		RewardBase:                  uint64Ptr(0),
		Rewards:                     account.Rewards,
		Round:                       account.Round,
		Status:                      account.Status,
		Type:                        strPtr("unknown"),
	}

	return ret
}

func fetchAccounts(options idb.AccountQueryOptions, atRound *uint64, ctx context.Context) ([]generated.Account, error) {
	accountchan := IndexerDb.GetAccounts(ctx, options)

	accounts := make([]generated.Account, 0)
	for actrow := range accountchan {
		if actrow.Error != nil {
			return nil, actrow.Error
		}

		fmt.Printf("object: %v\n", actrow)
		fmt.Printf("amt: %d\n", actrow.Account.Amount)
		fmt.Printf("round: %d\n", actrow.Account.Round)

		// Compute for a given round if requested.
		var account generated.Account
		if atRound != nil {
			acct, err := accounting.AccountAtRound(actrow.Account, *atRound, IndexerDb)
			if err != nil {
				return nil, fmt.Errorf("problem computing account at round: %v", err)
			}
			account = accountToAccount(acct)
		} else {
			account = accountToAccount(actrow.Account)
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// TODO: Replace with lsig.Blank() when that gets merged into go-algorand-sdk
func isBlank(lsig sdk_types.LogicSig) bool {
	if lsig.Args != nil {
		return false
	}
	if len(lsig.Logic) != 0 {
		return false
	}
	if !lsig.Msig.Blank() {
		return false
	}
	if lsig.Sig != (sdk_types.Signature{}) {
		return false
	}
	return true
}

func sigToTransactionSig(sig sdk_types.Signature) *string {
	if sig == (sdk_types.Signature{}) {
		return nil
	}
	tsig := base64.StdEncoding.EncodeToString(sig[:])
	return &tsig
}

func msigToTransactionMsig(msig sdk_types.MultisigSig) *generated.TransactionSignatureMultisig {
	if msig.Blank() {
		return nil
	}

	subsigs := make([]generated.TransactionSignatureMultisigSubsignature, 0)
	for _, subsig := range msig.Subsigs {
		subsigs = append(subsigs, generated.TransactionSignatureMultisigSubsignature{
			PublicKey: strPtr(base64.StdEncoding.EncodeToString(subsig.Key[:])),
			Signature: sigToTransactionSig(subsig.Sig),
		})
	}

	ret := generated.TransactionSignatureMultisig{
		Subsignature: &subsigs,
		Threshold:    uint64Ptr(uint64(msig.Threshold)),
		Version:      uint64Ptr(uint64(msig.Version)),
	}
	return &ret
}

func lsigToTransactionLsig(lsig sdk_types.LogicSig) *generated.TransactionSignatureLogicsig {
	if isBlank(lsig) {
		return nil
	}

	args := make([]string, 0)
	for _, arg := range lsig.Args {
		args = append(args, base64.StdEncoding.EncodeToString(arg))
	}

	ret := generated.TransactionSignatureLogicsig{
		Args:              &args,
		Logic:             strPtr(base64.StdEncoding.EncodeToString(lsig.Logic)),
		MultisigSignature: msigToTransactionMsig(lsig.Msig),
		Signature:         sigToTransactionSig(lsig.Sig),
	}

	return &ret
}

func txnRowToTransaction(row idb.TxnRow, gen genesis) (generated.Transaction, error) {
	if row.Error != nil {
		return generated.Transaction{}, row.Error
	}

	var stxn types.SignedTxnInBlock
	err := msgpack.Decode(row.TxnBytes, &stxn)
	if err != nil {
		return generated.Transaction{}, fmt.Errorf("error decoding transaction bytes: %s", err.Error())
	}

	var payment *generated.TransactionPayment
	var keyreg *generated.TransactionKeyreg
	var assetConfig *generated.TransactionAssetConfig
	var assetFreeze *generated.TransactionAssetFreeze
	var assetTransfer *generated.TransactionAssetTransfer

	switch stxn.Txn.Type {
	case sdk_types.PaymentTx:
		p := generated.TransactionPayment{
			Amount:           uint64(stxn.Txn.Amount),
			// TODO: Compute this data from somewhere?
			CloseAmount:      uint64Ptr(0),
			CloseRemainderTo: strPtr(stxn.Txn.CloseRemainderTo.String()),
			Receiver:         stxn.Txn.Receiver.String(),
		}
		payment = &p
	case sdk_types.KeyRegistrationTx:
		k := generated.TransactionKeyreg{
			NonParticipation:          boolPtr(stxn.Txn.Nonparticipation),
			SelectionParticipationKey: strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.SelectionPK[:])),
			VoteFirstValid:            uint64Ptr(uint64(stxn.Txn.VoteFirst)),
			VoteLastValid:             uint64Ptr(uint64(stxn.Txn.VoteLast)),
			VoteKeyDilution:           uint64Ptr(stxn.Txn.VoteKeyDilution),
			VoteParticipationKey:      strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.VotePK[:])),
		}
		keyreg = &k
	case sdk_types.AssetConfigTx:
		assetParams := generated.AssetParams{
			Clawback:      strPtr(stxn.Txn.AssetParams.Clawback.String()),
			Creator:       stxn.Txn.Sender.String(),
			Decimals:      uint64(stxn.Txn.AssetParams.Decimals),
			DefaultFrozen: boolPtr(stxn.Txn.AssetParams.DefaultFrozen),
			Freeze:        strPtr(stxn.Txn.AssetParams.Freeze.String()),
			Manager:       strPtr(stxn.Txn.AssetParams.Manager.String()),
			MetadataHash:  strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.AssetParams.MetadataHash[:])),
			Name:          strPtr(stxn.Txn.AssetParams.AssetName),
			Reserve:       strPtr(stxn.Txn.AssetParams.Reserve.String()),
			Total:         stxn.Txn.AssetParams.Total,
			UnitName:      strPtr(stxn.Txn.AssetParams.UnitName),
			Url:           strPtr(stxn.Txn.AssetParams.URL),
		}
		config := generated.TransactionAssetConfig{
			AssetId: nil,
			Params:  &assetParams,
		}
		assetConfig = &config
	case sdk_types.AssetTransferTx:
		t := generated.TransactionAssetTransfer{
			Amount:   stxn.Txn.AssetAmount,
			AssetId:  uint64(stxn.Txn.XferAsset),
			CloseTo:  strPtr(stxn.Txn.AssetCloseTo.String()),
			Receiver: stxn.Txn.AssetReceiver.String(),
			Sender:   strPtr(stxn.Txn.AssetSender.String()),
		}
		assetTransfer = &t
	case sdk_types.AssetFreezeTx:
		f := generated.TransactionAssetFreeze{
			Address:         stxn.Txn.FreezeAccount.String(),
			AssetId:         uint64(stxn.Txn.FreezeAsset),
			NewFreezeStatus: stxn.Txn.AssetFrozen,
		}
		assetFreeze = &f
	}

	sig := generated.TransactionSignature{
		Logicsig: lsigToTransactionLsig(stxn.Lsig),
		Multisig: msigToTransactionMsig(stxn.Msig),
		Sig:      sigToTransactionSig(stxn.Sig),
	}

	txn := generated.Transaction{
		AssetConfigTransaction:   assetConfig,
		AssetFreezeTransaction:   assetFreeze,
		AssetTransferTransaction: assetTransfer,
		PaymentTransaction:       payment,
		KeyregTransaction:        keyreg,
		ClosingAmount:            uint64Ptr(uint64(stxn.ClosingAmount)),
		ConfirmedRound:           uint64Ptr(row.Round),
		Fee:                      uint64Ptr(uint64(stxn.Txn.Fee)),
		FirstValid:               uint64Ptr(uint64(stxn.Txn.FirstValid)),
		GenesisHash:              nil, // This is removed from the stxn
		GenesisId:                nil, // This is removed from the stxn
		Group:                    strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.Group[:])),
		LastValid:                uint64Ptr(uint64(stxn.Txn.LastValid)),
		Lease:                    strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.Lease[:])),
		Note:                     strPtr(base64.StdEncoding.EncodeToString(stxn.Txn.Note[:])),
		Sender:                   strPtr(stxn.Txn.Sender.String()),
		ReceiverRewards:          uint64Ptr(uint64(stxn.ReceiverRewards)),
		CloseRewards:             uint64Ptr(uint64(stxn.CloseRewards)),
		SenderRewards:            uint64Ptr(uint64(stxn.SenderRewards)),
		Type:                     strPtr(string(stxn.Txn.Type)),
		Signature:                &sig,
		CreatedAssetIndex:        nil, // TODO: What is this?
		Id:                       strPtr(crypto.TransactionIDString(stxn.Txn)),
		PoolError:                nil, // TODO: What is this?
	}

	// Add in the genesis fields
	if stxn.HasGenesisHash {
		txn.GenesisHash = strPtr(base64.StdEncoding.EncodeToString(gen.genesisHash))
	}
	if stxn.HasGenesisID {
		txn.GenesisHash = strPtr(gen.genesisID)
	}

	return txn, nil
}

func decodeB64String(str *string, field string, errorArr []string) ([]byte, []string) {
	if str != nil {
		value, err := b64decode(*str)
		if err != nil {
			return nil, append(errorArr, fmt.Sprintf("unable to decode '%s': %s", field, err.Error()))
		}
		return value, errorArr
	}
	// Pass through
	return nil, errorArr
}

func decodeTimeString(str *string, field string, errorArr []string) (time.Time, []string) {
	if str != nil {
		value, err := parseTime(*str)
		if err != nil {
			return time.Time{}, append(errorArr, fmt.Sprintf("unable to decode '%s': %s", field, err.Error()))
		}
		value = value.In(time.FixedZone("UTC", 0))
		return value, errorArr
	}
	// Pass through
	return time.Time{}, errorArr
}

func decodeSigType(str *string, field string, errorArr []string) (string, []string) {
	if str != nil {
		sigTypeLc := strings.ToLower(*str)
		if _, ok := sigTypeEnumMap[sigTypeLc]; ok {
			return sigTypeLc, errorArr
		} else {
			return "", append(errorArr, fmt.Sprintf("invalid sigtype: '%s'", sigTypeLc))
		}
	}
	// Pass through
	return "", errorArr
}

func decodeType(str *string, field string, errorArr []string) (t int, err []string) {
	if str != nil {
		typeLc := strings.ToLower(*str)
		if val, ok := importer.TypeEnumMap[typeLc]; ok {
			return val, errorArr
		} else {
			return 0, append(errorArr, fmt.Sprintf("invalid transaction type: '%s'", typeLc))
		}
	}
	// Pass through
	return 0, errorArr
}


func transactionParamsToTransactionFilter(params generated.SearchForTransactionsParams) (filter idb.TransactionFilter, err error) {
	var errorArr = make([]string, 0)

	// Integer
	filter.MaxRound = uintOrDefault(params.MaxRound, 0)
	filter.MinRound = uintOrDefault(params.MinRound, 0)
	filter.AssetId = uintOrDefault(params.AssetId, 0)
	filter.Limit = uintOrDefault(params.Limit, 0)
	filter.Offset = params.Offset
	filter.Round = params.Round

	// Byte array
	filter.NotePrefix, errorArr = decodeB64String(params.Noteprefix, "note-prefix", errorArr)
	filter.Txid, errorArr = decodeB64String(params.Txid, "txid", errorArr)

	// Time
	filter.AfterTime, errorArr = decodeTimeString(params.AfterTime, "after-time", errorArr)
	filter.BeforeTime, errorArr = decodeTimeString(params.BeforeTime, "before-time", errorArr)

	// Enum
	filter.SigType, errorArr = decodeSigType(params.Sigtype, "sigtype", errorArr)
	filter.TypeEnum, errorArr = decodeType(params.Type, "type", errorArr)

	// If there were any errorArr while setting up the TransactionFilter, return now.
	if len(errorArr) > 0 {
		err = errors.New(strings.Join(errorArr, ", "))
	}

	return
}

// fetchTransactions is used to query the backend for transactions.
func fetchTransactions(filter idb.TransactionFilter, ctx context.Context) ([]generated.Transaction, error) {
	genesis := getGenesis(ctx)
	results := make([]generated.Transaction, 0)
	txchan := IndexerDb.Transactions(ctx, filter)
	for txrow := range txchan {
		tx, err := txnRowToTransaction(txrow, genesis)
		if err != nil {
			return nil, err
		}
		results = append(results, tx)
	}

	return results, nil
}

// (GET /account/{account-id})
func (si *ServerImplementation) LookupAccountByID(ctx echo.Context, accountId string, params generated.LookupAccountByIDParams) error {
	addr, err := sdk_types.DecodeAddress(accountId)
	if err != nil {
		return badRequest(ctx, fmt.Sprintf("Unable to parse address: %v", err))
	}

	options := idb.AccountQueryOptions {
			EqualToAddress:       addr[:],
			IncludeAssetHoldings: true,
			IncludeAssetParams:   true,
			Limit:                1,
	}

	accounts, err := fetchAccounts(options, params.Round, ctx.Request().Context())

	if err != nil {
		return badRequest(ctx, fmt.Sprintf("Failed while searching for account: %v", err))
	}

	if len(accounts) == 0 {
		return badRequest(ctx, fmt.Sprintf("No accounts found for address: %s", accountId))
	}

	if len(accounts) > 1 {
		return badRequest(ctx, fmt.Sprintf("Multiple accounts found for address, this shouldn't have happened: %s", accountId))
	}

	return ctx.JSON(http.StatusOK, generated.AccountResponse(accounts[0]))
}

// TODO: Missing filters:
//  * GreaterThan and LessThan account balances.
//  * Holds assetID
// TODO: "at round" is missing from these params, maybe it's fine to leave it out here.
// (GET /accounts)
func (si *ServerImplementation) SearchAccounts(ctx echo.Context, params generated.SearchAccountsParams) error {
	options := idb.AccountQueryOptions {
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		Limit:                uintOrDefault(params.Limit, 0),
	}

	accounts, err := fetchAccounts(options, nil, ctx.Request().Context())

	if err != nil {
		return badRequest(ctx,  fmt.Sprintf("Failed while searching for account: %v", err))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	response := generated.AccountsResponse{
		Accounts: accounts,
		Round:    round,
		Total:    0,
	}

	return ctx.JSON(http.StatusOK, response)
}

// (GET /account/{account-id}/transactions)
func (si *ServerImplementation) LookupAccountTransactions(ctx echo.Context, accountId string, params generated.LookupAccountTransactionsParams) error {
	// TODO
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id})
func (si *ServerImplementation) LookupAssetByID(ctx echo.Context, assetId uint64) error {
	// TODO
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id}/balances)
func (si *ServerImplementation) LookupAssetBalances(ctx echo.Context, assetId uint64, params generated.LookupAssetBalancesParams) error {
	// TODO: I don't think this exists in the backend yet.
	return errors.New("Unimplemented")
}

// (GET /asset/{asset-id}/transactions)
func (si *ServerImplementation) LookupAssetTransactions(ctx echo.Context, assetId uint64, params generated.LookupAssetTransactionsParams) error {
	// TODO: convert to /transaction call
	return errors.New("Unimplemented")
}

// (GET /assets)
func (si *ServerImplementation) SearchForAssets(ctx echo.Context, params generated.SearchForAssetsParams) error {
	// TODO: Need to implement 'fetchAssets'
	return errors.New("Unimplemented")
}

// (GET /block/{round-number})
func (si *ServerImplementation) LookupBlock(ctx echo.Context, roundNumber uint64) error {
	// TODO: Need to implement 'fetchBlock'
	return errors.New("Unimplemented")
}

// (GET /blocktimes)
func (si *ServerImplementation) LookupBlockTimes(ctx echo.Context) error {
	return errors.New("Unimplemented")
}

// TODO:
//  * Address - Sender/Receiver/CloseTo?
//  * MinAlgos - MaxAlgos? Min/Max asset? Or Min/Max with implicit MinAlgo/MinAsset?
//  * Implement "format", maybe that just returns raw bytes? Does it need to convert to stxn and add the genhash/genID back in first?
// (GET /transactions)
func (si *ServerImplementation) SearchForTransactions(ctx echo.Context, params generated.SearchForTransactionsParams) error {
	filter, err := transactionParamsToTransactionFilter(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, err := fetchTransactions(filter, ctx.Request().Context())

	if err != nil {
		return badRequest(ctx, fmt.Sprintf("error while searching for transactions: %v", err))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	response := generated.TransactionsResponse{
		Round:        &round,
		// TODO: Remove total
		Total:        uint64Ptr(0),
		Transactions: &txns,
	}

	return ctx.JSON(http.StatusOK, response)
}
