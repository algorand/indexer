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
	"strconv"
	"strings"
	"time"

	"github.com/algorand/indexer/api/generated"
	"github.com/algorand/indexer/idb"
	"github.com/labstack/echo/v4"
)

type ServerImplementation struct{
	EnableAddressSearchRoundRewind bool
}

func badRequest(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusBadRequest, generated.Error{
		Error: err,
	})
}

func indexerError(ctx echo.Context, err string) error {
	return ctx.JSON(http.StatusInternalServerError, generated.Error{
		Error: err,
	})
}

////////////////////////////////
// Safe dereference wrappers. //
////////////////////////////////
func uintOrDefault(x *uint64, def uint64) uint64 {
	if x != nil {
		return *x
	}
	return def
}

func uintOrDefaultMod(x *uint64, modifier int64, def uint64) uint64 {
	if x != nil {
		val := int64(*x) + modifier
		if val < 0 {
			return 0
		}
		return uint64(0)
	}
	return def
}

func strOrDefault(str *string) string {
	if str != nil {
		return *str
	}
	return ""
}

////////////////////////////
// Safe pointer wrappers. //
////////////////////////////
func uint64Ptr(x uint64) *uint64 {
	return &x
}

func bytePtr(x []byte) *[]byte {
	if len(x) == 0 {
		return nil
	}

	// Don't return if it's all zero.
	for _, v := range x {
		if v != 0 {
			return &x
		}
	}

	return nil
}

func timePtr(x time.Time) *time.Time {
	if x.IsZero() {
		return nil
	}
	return &x
}

func addrPtr(x sdk_types.Address) *string {
	if bytePtr(x[:]) == nil {
		return nil
	}
	return strPtr(x.String())
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
	genesisID   string
}

//////////////////////////////////////////////////////////////////////
// String decoding helpers (with 'errorArr' helper to group errors) //
//////////////////////////////////////////////////////////////////////

// TODO: This might be deprecated now.
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

// TODO: This might be deprecated now.
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

func decodeAddress(str *string, field string, errorArr []string) ([]byte, []string) {
	if str != nil {
		addr, err := sdk_types.DecodeAddress(*str)
		if err != nil {
			return nil, append(errorArr, fmt.Sprintf("Unable to parse address: %v", err))
		}
		return addr[:], errorArr
	}
	// Pass through
	return nil, errorArr
}

func decodeAddressRole(role *string, excludeCloseTo *bool, errorArr []string) (uint64, []string) {
	ret := uint64(0)

	// Set sender/receiver bits
	if role != nil {
		lc := strings.ToLower(*role)
		if lc == "sender" {
			ret |= idb.AddressRoleSender|idb.AddressRoleAssetSender
		} else if lc == "receiver" {
			ret |= idb.AddressRoleReceiver|idb.AddressRoleAssetReceiver

			// Also add close to flags to sender unless they were explicitly disabled.
			if excludeCloseTo == nil || !(*excludeCloseTo) {
				ret |= idb.AddressRoleCloseRemainderTo|idb.AddressRoleAssetCloseTo
			}
		} else if lc == "freeze-target" {
			ret |= idb.AddressRoleFreeze
		} else {
			return 0, append(errorArr, fmt.Sprintf("unknown address role: '%s' (expected sender, receiver or freeze-target)", lc))
		}
	}

	return ret, errorArr
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

////////////////////////////////////////////////////
// Helpers to convert to and from generated types //
////////////////////////////////////////////////////

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
			MetadataHash:  bytePtr(params.MetadataHash),
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
		SelectionParticipationKey: nil,
		VoteFirstValid:            uint64Ptr(0),
		VoteLastValid:             uint64Ptr(0),
		VoteKeyDilution:           uint64Ptr(0),
		VoteParticipationKey:      nil,
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
		Type:                        strPtr("unknown"), // TODO: how to get this?
	}

	return ret
}

func sigToTransactionSig(sig sdk_types.Signature) *[]byte {
	if sig == (sdk_types.Signature{}) {
		return nil
	}

	tsig := sig[:]
	return &tsig
}

func msigToTransactionMsig(msig sdk_types.MultisigSig) *generated.TransactionSignatureMultisig {
	if msig.Blank() {
		return nil
	}

	subsigs := make([]generated.TransactionSignatureMultisigSubsignature, 0)
	for _, subsig := range msig.Subsigs {
		subsigs = append(subsigs, generated.TransactionSignatureMultisigSubsignature{
			PublicKey: bytePtr(subsig.Key[:]),
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
		Logic:             lsig.Logic,
		MultisigSignature: msigToTransactionMsig(lsig.Msig),
		Signature:         sigToTransactionSig(lsig.Sig),
	}

	return &ret
}

func txnRowToTransaction(row idb.TxnRow) (generated.Transaction, error) {
	if row.Error != nil {
		return generated.Transaction{}, row.Error
	}

	var stxn types.SignedTxnWithAD
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
			CloseAmount:      uint64Ptr(row.Extra.AssetCloseAmount),
			CloseRemainderTo: addrPtr(stxn.Txn.CloseRemainderTo),
			Receiver:         stxn.Txn.Receiver.String(),
		}
		payment = &p
	case sdk_types.KeyRegistrationTx:
		k := generated.TransactionKeyreg{
			NonParticipation:          boolPtr(stxn.Txn.Nonparticipation),
			SelectionParticipationKey: bytePtr(stxn.Txn.SelectionPK[:]),
			VoteFirstValid:            uint64Ptr(uint64(stxn.Txn.VoteFirst)),
			VoteLastValid:             uint64Ptr(uint64(stxn.Txn.VoteLast)),
			VoteKeyDilution:           uint64Ptr(stxn.Txn.VoteKeyDilution),
			VoteParticipationKey:      bytePtr(stxn.Txn.VotePK[:]),
		}
		keyreg = &k
	case sdk_types.AssetConfigTx:
		assetParams := generated.AssetParams{
			Clawback:      addrPtr(stxn.Txn.AssetParams.Clawback),
			Creator:       stxn.Txn.Sender.String(),
			Decimals:      uint64(stxn.Txn.AssetParams.Decimals),
			DefaultFrozen: boolPtr(stxn.Txn.AssetParams.DefaultFrozen),
			Freeze:        addrPtr(stxn.Txn.AssetParams.Freeze),
			Manager:       addrPtr(stxn.Txn.AssetParams.Manager),
			MetadataHash:  bytePtr(stxn.Txn.AssetParams.MetadataHash[:]),
			Name:          strPtr(stxn.Txn.AssetParams.AssetName),
			Reserve:       addrPtr(stxn.Txn.AssetParams.Reserve),
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
			CloseTo:  addrPtr(stxn.Txn.AssetCloseTo),
			Receiver: stxn.Txn.AssetReceiver.String(),
			Sender:   addrPtr(stxn.Txn.AssetSender),
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
		IntraRoundOffset:         uint64Ptr(uint64(row.Intra)),
		RoundTime:                uint64Ptr(uint64(row.RoundTime.Unix())),
		Fee:                      uint64(stxn.Txn.Fee),
		FirstValid:               uint64(stxn.Txn.FirstValid),
		GenesisHash:              bytePtr(stxn.SignedTxn.Txn.GenesisHash[:]),
		GenesisId:                strPtr(stxn.SignedTxn.Txn.GenesisID),
		Group:                    bytePtr(stxn.Txn.Group[:]),
		LastValid:                uint64(stxn.Txn.LastValid),
		Lease:                    bytePtr(stxn.Txn.Lease[:]),
		Note:                     bytePtr(stxn.Txn.Note[:]),
		Sender:                   stxn.Txn.Sender.String(),
		ReceiverRewards:          uint64Ptr(uint64(stxn.ReceiverRewards)),
		CloseRewards:             uint64Ptr(uint64(stxn.CloseRewards)),
		SenderRewards:            uint64Ptr(uint64(stxn.SenderRewards)),
		Type:                     string(stxn.Txn.Type),
		Signature:                sig,
		CreatedAssetIndex:        nil,                            // TODO: What is this?
		Id:                       crypto.TransactionID(stxn.Txn), // TODO: This needs to come from the DB because of the GenesisHash / GenesisID
		PoolError:                nil,                            // TODO: What is this?
	}

	return txn, nil
}

func assetParamsToAssetQuery(params generated.SearchForAssetsParams) (idb.AssetsQuery, error) {
	creator, errorArr := decodeAddress(params.Creator, "creator", make([]string, 0))
	if len(errorArr) != 0 {
		return idb.AssetsQuery{}, errors.New(errorArr[0])
	}

	var assetGreaterThan uint64 = 0
	if params.Next != nil {
		agt, err := strconv.ParseUint(*params.Next, 10, 64)
		if err != nil {
			return idb.AssetsQuery{}, fmt.Errorf("unable to parse 'next': %v", err)
		}
		assetGreaterThan = agt
	}

	query := idb.AssetsQuery{
		AssetId:            uintOrDefault(params.AssetId, 0),
		AssetIdGreaterThan: assetGreaterThan,
		Creator:            creator,
		Name:				strOrDefault(params.Name),
		Unit:               strOrDefault(params.Unit),
		Query:              "",
		Limit:              uintOrDefault(params.Limit, 0),
	}

	return query, nil
}

// TODO: idb.TransactionFilter missing:
//      * MinAssetAmount
//      * MaxAssetAmount
// TODO: Convert Max/Min to LessThan/GreaterThan
// TODO: Pagination
func transactionParamsToTransactionFilter(params generated.SearchForTransactionsParams) (filter idb.TransactionFilter, err error) {
	var errorArr = make([]string, 0)

	if params.Round != nil && params.MaxRound != nil && *params.Round > *params.MaxRound {
		errorArr = append(errorArr, "invalid parameters: round > max-round")
	}

	if params.Round != nil && params.MinRound != nil && *params.Round < *params.MinRound {
		errorArr = append(errorArr, "invalid parameters: round < min-round")
	}

	// Integer
	filter.MaxRound = uintOrDefault(params.MaxRound, 0)
	filter.MinRound = uintOrDefault(params.MinRound, 0)
	filter.AssetId = uintOrDefault(params.AssetId, 0)
	filter.Limit = uintOrDefault(params.Limit, 0)
	// TODO: Convert Max/Min to LessThan/GreaterThan
	filter.MaxAlgos = uintOrDefaultMod(params.CurrencyLessThan, 1, 0)
	filter.MinAlgos = uintOrDefaultMod(params.CurrencyGreaterThan, -1, 0)
	filter.Round = params.Round
	//filter.Offset = params.Offset

	// String
	filter.AddressRole, errorArr = decodeAddressRole(params.AddressRole, params.ExcludeCloseTo, errorArr)

	// Address
	filter.Address, errorArr = decodeAddress(params.Address, "address", errorArr)

	// Byte array
	if params.NotePrefix != nil {
		filter.NotePrefix = *params.NotePrefix
	}
	if params.TxId != nil {
		filter.Txid = *params.TxId
	}

	// Time
	if params.AfterTime != nil {
		filter.AfterTime = *params.AfterTime
	}
	if params.BeforeTime != nil {
		filter.BeforeTime = *params.BeforeTime
	}

	// Enum
	filter.SigType, errorArr = decodeSigType(params.SigType, "sigtype", errorArr)
	filter.TypeEnum, errorArr = decodeType(params.TxType, "type", errorArr)

	// If there were any errorArr while setting up the TransactionFilter, return now.
	if len(errorArr) > 0 {
		err = errors.New(strings.Join(errorArr, ", "))
	}

	return
}

///////////////////////
// IndexerDb helpers //
///////////////////////

func fetchAssets(options idb.AssetsQuery, ctx context.Context) ([]generated.Asset, error) {
	assetchan := IndexerDb.Assets(ctx, options)
	assets := make([]generated.Asset, 0)
	for row := range assetchan {
		if row.Error != nil {
			return nil, row.Error
		}

		creator := sdk_types.Address{}
		if len(row.Creator) != len(creator) {
			return nil, fmt.Errorf("found an invalid creator address")
		}
		copy(creator[:], row.Creator[:])

		asset := generated.Asset{
			Index:  row.AssetId,
			Params: generated.AssetParams{
				Creator:       creator.String(),
				Name:          strPtr(row.Params.AssetName),
				UnitName:      strPtr(row.Params.UnitName),
				Url:           strPtr(row.Params.URL),
				Total:         row.Params.Total,
				Decimals:      uint64(row.Params.Decimals),
				DefaultFrozen: boolPtr(row.Params.DefaultFrozen),
				MetadataHash:  bytePtr(row.Params.MetadataHash[:]),
				Clawback:      strPtr(row.Params.Clawback.String()),
				Reserve:       strPtr(row.Params.Reserve.String()),
				Freeze:        strPtr(row.Params.Freeze.String()),
				Manager:       strPtr(row.Params.Manager.String()),
			},
		}

		assets = append(assets, asset)
	}
	return  assets, nil
}

func fetchAssetBalances(options idb.AssetBalanceQuery, ctx context.Context) ([]generated.MiniAssetHolding, error) {
	assetbalchan := IndexerDb.AssetBalances(ctx, options)
	balances := make([]generated.MiniAssetHolding, 0)
	for row := range assetbalchan {
		if row.Error != nil {
			return nil, row.Error
		}

		addr := sdk_types.Address{}
		if len(row.Address) != len(addr) {
			return nil, fmt.Errorf("found an invalid creator address")
		}
		copy(addr[:], row.Address[:])

		bal := generated.MiniAssetHolding{
			Address:  addr.String(),
			Amount:   row.Amount,
			IsFrozen: row.Frozen,
		}

		balances = append(balances, bal)
	}

	return balances, nil
}

func fetchBlock(round uint64) (generated.Block, error) {
	blk, err := IndexerDb.GetBlock(round)
	if err != nil {
		return generated.Block{}, fmt.Errorf("error while looking up for block for round '%d': %v", round, err)
	}

	rewards := generated.BlockRewards{
		FeeSink:                 "",
		RewardsCalculationRound: uint64(blk.RewardsRecalculationRound),
		RewardsLevel:            blk.RewardsLevel,
		RewardsPool:             blk.RewardsPool.String(),
		RewardsRate:             blk.RewardsRate,
		RewardsResidue:          blk.RewardsResidue,
	}

	upgradeState := generated.BlockUpgradeState{
		CurrentProtocol:        string(blk.CurrentProtocol),
		NextProtocol:           strPtr(string(blk.NextProtocol)),
		NextProtocolApprovals:  uint64Ptr(blk.NextProtocolApprovals),
		NextProtocolSwitchOn:   uint64Ptr(uint64(blk.NextProtocolSwitchOn)),
		NextProtocolVoteBefore: uint64Ptr(uint64(blk.NextProtocolVoteBefore)),
	}

	upgradeVote := generated.BlockUpgradeVote{
		UpgradeApprove: boolPtr(blk.UpgradeApprove),
		UpgradeDelay:   uint64Ptr(uint64(blk.UpgradeDelay)),
		UpgradePropose: strPtr(string(blk.UpgradePropose)),
	}

	ret := generated.Block{
		GenesisHash:       blk.GenesisHash[:],
		GenesisId:         blk.GenesisID,
		PreviousBlockHash: blk.Branch[:],
		Rewards:           &rewards,
		Round:             uint64(blk.Round),
		Seed:              blk.Seed[:],
		Timestamp:         uint64(blk.TimeStamp),
		Transactions:      nil,
		TransactionsRoot:  blk.TxnRoot[:],
		TxnCounter:        uint64Ptr(blk.TxnCounter),
		UpgradeState:      &upgradeState,
		UpgradeVote:       &upgradeVote,
	}

	return ret, nil
}

func fetchAccounts(options idb.AccountQueryOptions, atRound *uint64, ctx context.Context) ([]generated.Account, error) {
	accountchan := IndexerDb.GetAccounts(ctx, options)

	accounts := make([]generated.Account, 0)
	for row := range accountchan {
		if row.Error != nil {
			return nil, row.Error
		}

		fmt.Printf("object: %v\n", row)
		fmt.Printf("amt: %d\n", row.Account.Amount)
		fmt.Printf("round: %d\n", row.Account.Round)

		// Compute for a given round if requested.
		var account generated.Account
		if atRound != nil {
			acct, err := accounting.AccountAtRound(row.Account, *atRound, IndexerDb)
			if err != nil {
				return nil, fmt.Errorf("problem computing account at round: %v", err)
			}
			account = accountToAccount(acct)
		} else {
			account = accountToAccount(row.Account)
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// fetchTransactions is used to query the backend for transactions.
func fetchTransactions(filter idb.TransactionFilter, ctx context.Context) ([]generated.Transaction, error) {
	results := make([]generated.Transaction, 0)
	txchan := IndexerDb.Transactions(ctx, filter)
	for txrow := range txchan {
		tx, err := txnRowToTransaction(txrow)
		if err != nil {
			return nil, err
		}
		results = append(results, tx)
	}

	return results, nil
}

/////////////////////////////
// Handler implementations //
/////////////////////////////

// (GET /account/{account-id})
func (si *ServerImplementation) LookupAccountByID(ctx echo.Context, accountId string, params generated.LookupAccountByIDParams) error {
	addr, errors := decodeAddress(&accountId, "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	options := idb.AccountQueryOptions{
		EqualToAddress:       addr[:],
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		Limit:                1,
	}

	accounts, err := fetchAccounts(options, params.Round, ctx.Request().Context())

	if err != nil {
		return indexerError(ctx, fmt.Sprintf("Failed while searching for account: %v", err))
	}

	if len(accounts) == 0 {
		return badRequest(ctx, fmt.Sprintf("No accounts found for address: %s", accountId))
	}

	if len(accounts) > 1 {
		return badRequest(ctx, fmt.Sprintf("Multiple accounts found for address, this shouldn't have happened: %s", accountId))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	return ctx.JSON(http.StatusOK, generated.AccountResponse{
		CurrentRound: round,
		Account:      accounts[0],
	})
}

// TODO: Missing filters:
//  * Holds assetID
//  * assetID holding gt/lt amount
// (GET /accounts)
func (si *ServerImplementation) SearchAccounts(ctx echo.Context, params generated.SearchAccountsParams) error {
	options := idb.AccountQueryOptions {
		AlgosGreaterThan:     uintOrDefault(params.CurrencyGreaterThan, 0),
		AlgosLessThan:        uintOrDefault(params.CurrencyLessThan, 0),
		IncludeAssetHoldings: true,
		IncludeAssetParams:   true,
		Limit:                uintOrDefault(params.Limit, 0),
	}

	var atRound *uint64

	if si.EnableAddressSearchRoundRewind {
		atRound = params.Round
	}

	if params.Next != nil {
		addr, err := sdk_types.DecodeAddress(*params.Next)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, "Unable to parse next.")
		}
		options.GreaterThanAddress = addr[:]
	}

	accounts, err := fetchAccounts(options, atRound, ctx.Request().Context())

	if err != nil {
		return badRequest(ctx, fmt.Sprintf("Failed while searching for account: %v", err))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	// Set the next token if we hit the results limit
	// TODO: set the limit to +1, so we know that there are actually more results?
	var next *string
	if params.Limit != nil && uint64(len(accounts)) >= *params.Limit {
		next = strPtr(accounts[len(accounts)-1].Address)
	}

	response := generated.AccountsResponse{
		CurrentRound: round,
		NextToken:    next,
		Accounts:     accounts,
	}

	return ctx.JSON(http.StatusOK, response)
}

// TODO: Pagination
// (GET /account/{account-id}/transactions)
func (si *ServerImplementation) LookupAccountTransactions(ctx echo.Context, accountId string, params generated.LookupAccountTransactionsParams) error {
	// Check that a valid account was provided
	_, errors := decodeAddress(strPtr(accountId), "account-id", make([]string, 0))
	if len(errors) != 0 {
		return badRequest(ctx, errors[0])
	}

	searchParams := generated.SearchForTransactionsParams{
		Address:             strPtr(accountId),
		// not applicable to this endpoint
		//AddressRole:         params.AddressRole,
		//ExcludeCloseTo:      params.ExcludeCloseTo,
		AssetId:			 params.AssetId,
		Limit:               params.Limit,
		Next:                params.Next,
		NotePrefix:          params.NotePrefix,
		TxType:              params.TxType,
		SigType:             params.SigType,
		TxId:                params.TxId,
		Round:               params.Round,
		MinRound:            params.MinRound,
		MaxRound:            params.MaxRound,
		BeforeTime:          params.BeforeTime,
		AfterTime:           params.AfterTime,
		CurrencyGreaterThan: params.CurrencyGreaterThan,
		CurrencyLessThan:    params.CurrencyLessThan,
	}

	return si.SearchForTransactions(ctx, searchParams)
}

// (GET /asset/{asset-id})
func (si *ServerImplementation) LookupAssetByID(ctx echo.Context, assetId uint64) error {
	search := generated.SearchForAssetsParams{
		AssetId: uint64Ptr(assetId),
		Limit:   uint64Ptr(1),
	}
	options, err := assetParamsToAssetQuery(search)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, err := fetchAssets(options, ctx.Request().Context())
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	if len(assets) == 0 {
		return badRequest(ctx, fmt.Sprintf("No assets found for id: %d", assetId))
	}

	if len(assets) > 1 {
		return badRequest(ctx, fmt.Sprintf("Multiple assets found for id, this shouldn't have happened: %s", assetId))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	return ctx.JSON(http.StatusOK, generated.AssetResponse{
		Asset:       assets[0],
		CurrentRound: round,
	})
}

// (GET /asset/{asset-id}/balances)
func (si *ServerImplementation) LookupAssetBalances(ctx echo.Context, assetId uint64, params generated.LookupAssetBalancesParams) error {
	query := idb.AssetBalanceQuery{
		AssetId:     assetId,
		MinAmount:   uintOrDefault(params.CurrencyGreaterThan, 0),
		MaxAmount:   uintOrDefault(params.CurrencyLessThan, 0),
		Limit:       uintOrDefault(params.Limit, 0),
	}

	if params.Next != nil {
		addr, err := sdk_types.DecodeAddress(*params.Next)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, "Unable to parse next.")
		}
		query.PrevAddress = addr[:]
	}

	balances, err := fetchAssetBalances(query, ctx.Request().Context())
	if err != nil {
		indexerError(ctx, err.Error())
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	// Set the next token if we hit the results limit
	// TODO: set the limit to +1, so we know that there are actually more results?
	var next *string
	if params.Limit != nil && uint64(len(balances)) >= *params.Limit {
		next = strPtr(balances[len(balances)-1].Address)
	}

	return ctx.JSON(http.StatusOK, generated.AssetBalancesResponse{
		CurrentRound: round,
		NextToken:    next,
		Balances:     balances,
	})
}

// TODO: pagination
// (GET /asset/{asset-id}/transactions)
func (si *ServerImplementation) LookupAssetTransactions(ctx echo.Context, assetId uint64, params generated.LookupAssetTransactionsParams) error {
	searchParams := generated.SearchForTransactionsParams{
		AssetId:             uint64Ptr(assetId),
		Limit:               params.Limit,
		Next:                params.Next,
		NotePrefix:          params.NotePrefix,
		TxType:              params.TxType,
		SigType:             params.SigType,
		TxId:                params.TxId,
		Round:               params.Round,
		MinRound:            params.MinRound,
		MaxRound:            params.MaxRound,
		BeforeTime:          params.BeforeTime,
		AfterTime:           params.AfterTime,
		CurrencyGreaterThan: params.CurrencyGreaterThan,
		CurrencyLessThan:    params.CurrencyLessThan,
		Address:             params.AddressRole,
		AddressRole:         params.AddressRole,
		ExcludeCloseTo:      params.ExcludeCloseTo,
	}

	return si.SearchForTransactions(ctx, searchParams)
}

// TODO: Fuzzy matching? check Name/Unit for asterisk and convert to query...
// (GET /assets)
func (si *ServerImplementation) SearchForAssets(ctx echo.Context, params generated.SearchForAssetsParams) error {
	options, err := assetParamsToAssetQuery(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	assets, err := fetchAssets(options, ctx.Request().Context())
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	// Set the next token if we hit the results limit
	// TODO: set the limit to +1, so we know that there are actually more results?
	var next *string
	if params.Limit != nil && uint64(len(assets)) >= *params.Limit {
		next = strPtr(strconv.FormatUint(assets[len(assets)-1].Index, 10))
	}

	return ctx.JSON(http.StatusOK, generated.AssetsResponse{
		CurrentRound: round,
		NextToken:    next,
		Assets:       assets,
	})
}

// (GET /block/{round-number})
func (si *ServerImplementation) LookupBlock(ctx echo.Context, roundNumber uint64) error {
	blk, err := fetchBlock(roundNumber)
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	// Lookup transactions
	filter := idb.TransactionFilter{ Round: uint64Ptr(roundNumber) }
	txns, err := fetchTransactions(filter, ctx.Request().Context())
	if err != nil {
		return indexerError(ctx, fmt.Sprintf("error while looking up for transactions for round '%d': %v", roundNumber, err))
	}

	blk.Transactions = &txns
	return ctx.JSON(http.StatusOK, generated.BlockResponse(blk))
}

// TODO:
//  * MinAssetAmount
//  * MaxAssetAmount
//  * Pagination
// (GET /transactions)
func (si *ServerImplementation) SearchForTransactions(ctx echo.Context, params generated.SearchForTransactionsParams) error {
	filter, err := transactionParamsToTransactionFilter(params)
	if err != nil {
		return badRequest(ctx, err.Error())
	}

	// Fetch the transactions
	txns, err := fetchTransactions(filter, ctx.Request().Context())

	if err != nil {
		return indexerError(ctx, fmt.Sprintf("error while searching for transactions: %v", err))
	}

	round, err := IndexerDb.GetMaxRound()
	if err != nil {
		return indexerError(ctx, err.Error())
	}

	response := generated.TransactionsResponse{
		CurrentRound: round,
		NextToken:    nil, // TODO
		Transactions: txns,
	}

	return ctx.JSON(http.StatusOK, response)
}
