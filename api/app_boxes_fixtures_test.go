package api

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/algorand/go-algorand/ledger/ledgercore"
	"github.com/algorand/indexer/idb/postgres"
	"github.com/stretchr/testify/require"

	"github.com/algorand/avm-abi/apps"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/indexer/util/test"
)

func goalEncode(t *testing.T, s string) string {
	b1, err := apps.NewAppCallBytes(s)
	require.NoError(t, err, s)
	b2, err := b1.Raw()
	require.NoError(t, err)
	return string(b2)
}

var goalEncodingExamples map[string]string = map[string]string{
	"str":         "str",
	"string":      "string",
	"int":         "42",
	"integer":     "100",
	"addr":        basics.AppIndex(3).Address().String(),
	"address":     basics.AppIndex(5).Address().String(),
	"b32":         base32.StdEncoding.EncodeToString([]byte("b32")),
	"base32":      base32.StdEncoding.EncodeToString([]byte("base32")),
	"byte base32": base32.StdEncoding.EncodeToString([]byte("byte base32")),
	"b64":         base64.StdEncoding.EncodeToString([]byte("b64")),
	"base64":      base64.StdEncoding.EncodeToString([]byte("base64")),
	"byte base64": base64.StdEncoding.EncodeToString([]byte("byte base64")),
	"abi":         `(uint64,string,bool[]):[399,"pls pass",[true,false]]`,
}

func setupLiveBoxes(t *testing.T, db *postgres.IndexerDb) {
	deleted := "DELETED"

	firstAppid := basics.AppIndex(1)
	thirdAppid := basics.AppIndex(5)

	// ---- ROUND 1: create and fund the box app and another app which won't have boxes ---- //
	//currentRound := basics.Round(1)

	//createTxn, err := test.MakeComplexCreateAppTxn(test.AccountA, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	//require.NoError(t, err)
	//
	//payNewAppTxn := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountA, firstAppid.Address(), basics.Address{},
	//	basics.Address{})
	//
	//createTxn2, err := test.MakeComplexCreateAppTxn(test.AccountB, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	//require.NoError(t, err)
	//payNewAppTxn2 := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountB, secondAppid.Address(), basics.Address{},
	//	basics.Address{})
	//
	//createTxn3, err := test.MakeComplexCreateAppTxn(test.AccountC, test.BoxApprovalProgram, test.BoxClearProgram, 8)
	//require.NoError(t, err)
	//payNewAppTxn3 := test.MakePaymentTxn(1000, 500000, 0, 0, 0, 0, test.AccountC, thirdAppid.Address(), basics.Address{},
	//	basics.Address{})

	vb1, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR1.vb")
	require.NoError(t, err)
	blk1 := ledgercore.MakeValidatedBlock(vb1.Blk, vb1.Delta)
	err = db.AddBlock(&blk1)
	require.NoError(t, err)

	// ---- ROUND 2: create 8 boxes for appid == 1  ---- //
	boxNames := []string{
		"a great box",
		"another great box",
		"not so great box",
		"disappointing box",
		"don't box me in this way",
		"I will be assimilated",
		"I'm destined for deletion",
		"box #8",
	}

	expectedAppBoxes := map[basics.AppIndex]map[string]string{}
	expectedAppBoxes[firstAppid] = map[string]string{}
	newBoxValue := "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//boxTxns := make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range boxNames {
	//	expectedAppBoxes[firstAppid][apps.MakeBoxKey(uint64(firstAppid), boxName)] = newBoxValue
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(firstAppid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//}

	for _, boxName := range boxNames {
		expectedAppBoxes[firstAppid][apps.MakeBoxKey(uint64(firstAppid), boxName)] = newBoxValue
	}

	vb2, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR2.vb")
	require.NoError(t, err)
	blk2 := ledgercore.MakeValidatedBlock(vb2.Blk, vb2.Delta)
	err = db.AddBlock(&blk2)
	require.NoError(t, err)

	// ---- ROUND 3: populate the boxes appropriately  ---- //
	appBoxesToSet := map[string]string{
		"a great box":               "it's a wonderful box",
		"another great box":         "I'm wonderful too",
		"not so great box":          "bummer",
		"disappointing box":         "RUG PULL!!!!",
		"don't box me in this way":  "non box-conforming",
		"I will be assimilated":     "THE BORG",
		"I'm destined for deletion": "I'm still alive!!!",
		"box #8":                    "eight is beautiful",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(firstAppid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(firstAppid), boxName)
	//	expectedAppBoxes[firstAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}

	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(firstAppid), boxName)
		expectedAppBoxes[firstAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}

	vb3, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR3.vb")
	require.NoError(t, err)
	blk3 := ledgercore.MakeValidatedBlock(vb3.Blk, vb3.Delta)
	err = db.AddBlock(&blk3)
	require.NoError(t, err)

	// ---- ROUND 4: delete the unhappy boxes  ---- //
	appBoxesToDelete := []string{
		"not so great box",
		"disappointing box",
		"I'm destined for deletion",
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToDelete {
	//	args := []string{"delete", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(firstAppid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(firstAppid), boxName)
	//	expectedAppBoxes[firstAppid][key] = deleted
	//}
	for _, boxName := range appBoxesToDelete {
		key := apps.MakeBoxKey(uint64(firstAppid), boxName)
		expectedAppBoxes[firstAppid][key] = deleted
	}

	vb4, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR4.vb")
	require.NoError(t, err)
	blk4 := ledgercore.MakeValidatedBlock(vb4.Blk, vb4.Delta)
	err = db.AddBlock(&blk4)
	require.NoError(t, err)

	// ---- ROUND 5: create 4 new boxes, overwriting one of the former boxes  ---- //
	randBoxName := []byte{0x52, 0xfd, 0xfc, 0x7, 0x21, 0x82, 0x65, 0x4f, 0x16, 0x3f, 0x5f, 0xf, 0x9a, 0x62, 0x1d, 0x72, 0x95, 0x66, 0xc7, 0x4d, 0x10, 0x3, 0x7c, 0x4d, 0x7b, 0xbb, 0x4, 0x7, 0xd1, 0xe2, 0xc6, 0x49, 0x81, 0x85, 0x5a, 0xd8, 0x68, 0x1d, 0xd, 0x86, 0xd1, 0xe9, 0x1e, 0x0, 0x16, 0x79, 0x39, 0xcb, 0x66, 0x94, 0xd2, 0xc4, 0x22, 0xac, 0xd2, 0x8, 0xa0, 0x7, 0x29, 0x39, 0x48, 0x7f, 0x69, 0x99}
	appBoxesToCreate := []string{
		"fantabulous",
		"disappointing box", // overwriting here
		"AVM is the new EVM",
		string(randBoxName),
	}
	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, boxName := range appBoxesToCreate {
	//	args := []string{"create", boxName}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(firstAppid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(firstAppid), boxName)
	//	expectedAppBoxes[firstAppid][key] = newBoxValue
	//}
	for _, boxName := range appBoxesToCreate {
		key := apps.MakeBoxKey(uint64(firstAppid), boxName)
		expectedAppBoxes[firstAppid][key] = newBoxValue
	}

	vb5, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR5.vb")
	require.NoError(t, err)
	blk5 := ledgercore.MakeValidatedBlock(vb5.Blk, vb5.Delta)
	err = db.AddBlock(&blk5)
	require.NoError(t, err)

	// ---- ROUND 6: populate the 4 new boxes  ---- //
	randBoxValue := []byte{0xeb, 0x9d, 0x18, 0xa4, 0x47, 0x84, 0x4, 0x5d, 0x87, 0xf3, 0xc6, 0x7c, 0xf2, 0x27, 0x46, 0xe9, 0x95, 0xaf, 0x5a, 0x25, 0x36, 0x79, 0x51, 0xba}
	appBoxesToSet = map[string]string{
		"fantabulous":        "Italian food's the best!", // max char's
		"disappointing box":  "you made it!",
		"AVM is the new EVM": "yes we can!",
		string(randBoxName):  string(randBoxValue),
	}
	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for boxName, valPrefix := range appBoxesToSet {
	//	args := []string{"set", boxName, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(firstAppid), test.AccountA, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(firstAppid), boxName)
	//	expectedAppBoxes[firstAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}
	for boxName, valPrefix := range appBoxesToSet {
		key := apps.MakeBoxKey(uint64(firstAppid), boxName)
		expectedAppBoxes[firstAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}

	vb6, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR6.vb")
	require.NoError(t, err)
	blk6 := ledgercore.MakeValidatedBlock(vb6.Blk, vb6.Delta)
	err = db.AddBlock(&blk6)
	require.NoError(t, err)

	// ---- ROUND 7: create GOAL-encoding boxes for appid == 5  ---- //
	encodingExamples := make(map[string]string, len(goalEncodingExamples))
	for k, v := range goalEncodingExamples {
		encodingExamples[k] = goalEncode(t, k+":"+v)
	}

	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//expectedAppBoxes[thirdAppid] = map[string]string{}
	//for _, boxName := range encodingExamples {
	//	args := []string{"create", boxName}
	//	expectedAppBoxes[thirdAppid][apps.MakeBoxKey(uint64(thirdAppid), boxName)] = newBoxValue
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(thirdAppid), test.AccountC, args, []string{boxName})
	//	boxTxns = append(boxTxns, &boxTxn)
	//}
	//
	//block, err = test.MakeBlockForTxns(blockHdr, boxTxns...)
	//require.NoError(t, err)
	expectedAppBoxes[thirdAppid] = map[string]string{}
	for _, boxName := range encodingExamples {
		expectedAppBoxes[thirdAppid][apps.MakeBoxKey(uint64(thirdAppid), boxName)] = newBoxValue
	}

	vb7, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR7.vb")
	require.NoError(t, err)
	blk7 := ledgercore.MakeValidatedBlock(vb7.Blk, vb7.Delta)
	err = db.AddBlock(&blk7)
	require.NoError(t, err)

	// ---- ROUND 8: populate GOAL-encoding boxes for appid == 5  ---- //
	//boxTxns = make([]*transactions.SignedTxnWithAD, 0)
	//for _, valPrefix := range encodingExamples {
	//	require.LessOrEqual(t, len(valPrefix), 40)
	//	args := []string{"set", valPrefix, valPrefix}
	//	boxTxn := test.MakeAppCallTxnWithBoxes(uint64(thirdAppid), test.AccountC, args, []string{valPrefix})
	//	boxTxns = append(boxTxns, &boxTxn)
	//
	//	key := apps.MakeBoxKey(uint64(thirdAppid), valPrefix)
	//	expectedAppBoxes[thirdAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	//}

	for _, valPrefix := range encodingExamples {
		require.LessOrEqual(t, len(valPrefix), 40)
		key := apps.MakeBoxKey(uint64(thirdAppid), valPrefix)
		expectedAppBoxes[thirdAppid][key] = valPrefix + newBoxValue[len(valPrefix):]
	}

	vb8, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR8.vb")
	require.NoError(t, err)
	blk8 := ledgercore.MakeValidatedBlock(vb8.Blk, vb8.Delta)
	err = db.AddBlock(&blk8)
	require.NoError(t, err)

	//---- ROUND 9: delete appid == 5 thus orphaning the boxes
	//deleteTxn := test.MakeAppDestroyTxn(uint64(thirdAppid), test.AccountC)
	//block, err = test.MakeBlockForTxns(blockHdr, &deleteTxn)
	//require.NoError(t, err)

	vb9, err := test.ReadValidatedBlockFromFile("test_resources/validated_blocks/LiveBoxesR9.vb")
	require.NoError(t, err)
	blk9 := ledgercore.MakeValidatedBlock(vb9.Blk, vb9.Delta)
	err = db.AddBlock(&blk9)
	require.NoError(t, err)

	// ---- SUMMARY ---- //

	totals := map[basics.AppIndex]map[string]int{}
	for appIndex, appBoxes := range expectedAppBoxes {
		totals[appIndex] = map[string]int{
			"tBoxes":    0,
			"tBoxBytes": 0,
		}
		for k, v := range appBoxes {
			if v != deleted {
				totals[appIndex]["tBoxes"]++
				totals[appIndex]["tBoxBytes"] += len(k) + len(v) - 11
			}
		}
	}

	// This is a manual sanity check only.
	// Validations of db and response contents prior to server response are tested elsewhere.
	// TODO: consider incorporating such stateful validations here as well.
	fmt.Printf("expectedAppBoxes=%+v\n", expectedAppBoxes)
	fmt.Printf("expected totals=%+v\n", totals)
}

var boxSeedFixture = fixture{
	File:   "boxes.json",
	Owner:  "TestBoxes",
	Frozen: true,
	Cases: []testCase{
		// /v2/accounts - 1 case
		{
			Name: "What are all the accounts?",
			Request: requestInfo{
				Path:   "/v2/accounts",
				Params: []param{},
			},
		},
		// /v2/applications - 1 case
		{
			Name: "What are all the apps?",
			Request: requestInfo{
				Path:   "/v2/applications",
				Params: []param{},
			},
		},
		// /v2/applications/:app-id - 4 cases
		{
			Name: "Lookup non-existing app 1337",
			Request: requestInfo{
				Path:   "/v2/applications/1337",
				Params: []param{},
			},
		},
		{
			Name: "Lookup app 3 (funded with no boxes)",
			Request: requestInfo{
				Path:   "/v2/applications/3",
				Params: []param{},
			},
		},
		{
			Name: "Lookup app 1 (funded with boxes)",
			Request: requestInfo{
				Path:   "/v2/applications/1",
				Params: []param{},
			},
		},
		{
			Name: "Lookup DELETED app 5 (funded with encoding test named boxes)",
			Request: requestInfo{
				Path:   "/v2/applications/5",
				Params: []param{},
			},
		},
		// /v2/accounts/:account-id - 1 non-app case and 2 cases using AppIndex.Address()
		{
			Name: "Creator account - not an app account - no params",
			Request: requestInfo{
				Path:   "/v2/accounts/LMTOYRT2WPSUY6JTCW2URER6YN3GETJ5FHTQBA55EVK66JG2QOB32WPIHY",
				Params: []param{},
			},
		},
		{
			Name: "App 3 (as account) totals no boxes - no params",
			Request: requestInfo{
				Path:   "/v2/accounts/" + basics.AppIndex(3).Address().String(),
				Params: []param{},
			},
		},
		{
			Name: "App 1 (as account) totals with boxes - no params",
			Request: requestInfo{
				Path:   "/v2/accounts/" + basics.AppIndex(1).Address().String(),
				Params: []param{},
			},
		},
		// /v2/applications/:app-id/boxes - 5 apps with lots of param variations
		{
			Name: "Boxes of a app with id == math.MaxInt64",
			Request: requestInfo{
				Path:   "/v2/applications/9223372036854775807/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of a app with id == math.MaxInt64 + 1",
			Request: requestInfo{
				Path:   "/v2/applications/9223372036854775808/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of a non-existing app 1337",
			Request: requestInfo{
				Path:   "/v2/applications/1337/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of app 3 with no boxes: no params",
			Request: requestInfo{
				Path:   "/v2/applications/3/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of DELETED app 5 with goal encoded boxes: no params",
			Request: requestInfo{
				Path:   "/v2/applications/5/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: no params",
			Request: requestInfo{
				Path:   "/v2/applications/1/boxes",
				Params: []param{},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - page 1",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - page 2 - b64",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", "b64:Uv38ByGCZU8WP18PmmIdcpVmx00QA3xNe7sEB9HixkmBhVrYaB0NhtHpHgAWeTnLZpTSxCKs0gigByk5SH9pmQ=="},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - page 3 - b64",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", "b64:Ym94ICM4"},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - MISSING b64 prefix",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", "Ym94ICM4"},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - goal app arg encoding str",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", "str:box #8"},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - page 4 (empty) - b64",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", "b64:ZmFudGFidWxvdXM="},
				},
			},
		},
		{
			Name: "Boxes of app 1 with boxes: limit 3 - ERROR because when next param provided -even empty string- it must be goal app arg encoded",
			Request: requestInfo{
				Path: "/v2/applications/1/boxes",
				Params: []param{
					{"limit", "3"},
					{"next", ""},
				},
			},
		},
		// /v2/applications/:app-id/box?name=...  - lots and lots
		{
			Name: "Boxes (with made up name param) of a app with id == math.MaxInt64",
			Request: requestInfo{
				Path: "/v2/applications/9223372036854775807/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "Box (with made up name param) of a app with id == math.MaxInt64 + 1",
			Request: requestInfo{
				Path: "/v2/applications/9223372036854775808/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},

		{
			Name: "A box attempt for a non-existing app 1337",
			Request: requestInfo{
				Path: "/v2/applications/1337/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "A box attempt for a non-existing app 1337 - without the required box name param",
			Request: requestInfo{
				Path:   "/v2/applications/1337/box",
				Params: []param{},
			},
		},
		{
			Name: "A box attempt for a existing app 3 - without the required box name param",
			Request: requestInfo{
				Path:   "/v2/applications/3/box",
				Params: []param{},
			},
		},
		{
			Name: "App 3 box (non-existing)",
			Request: requestInfo{
				Path: "/v2/applications/3/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "App 1 box (non-existing)",
			Request: requestInfo{
				Path: "/v2/applications/1/box",
				Params: []param{
					{"name", "string:non-existing"},
				},
			},
		},
		{
			Name: "App 1 box (a great box)",
			Request: requestInfo{
				Path: "/v2/applications/1/box",
				Params: []param{
					{"name", "string:a great box"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (str:str) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "str:str"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (integer:100) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "integer:100"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (base32:MJQXGZJTGI======) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "base32:MJQXGZJTGI======"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (b64:YjY0) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "b64:YjY0"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (base64:YmFzZTY0) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "base64:YmFzZTY0"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (string:string) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "string:string"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (int:42) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "int:42"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (abi:(uint64,string,bool[]):[399,\"pls pass\",[true,false]]) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "abi:(uint64,string,bool[]):[399,\"pls pass\",[true,false]]"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (addr:LMTOYRT2WPSUY6JTCW2URER6YN3GETJ5FHTQBA55EVK66JG2QOB32WPIHY) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "addr:LMTOYRT2WPSUY6JTCW2URER6YN3GETJ5FHTQBA55EVK66JG2QOB32WPIHY"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (address:2SYXFSCZAQCZ7YIFUCUZYOVR7G6Y3UBGSJIWT4EZ4CO3T6WVYTMHVSANOY) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "address:2SYXFSCZAQCZ7YIFUCUZYOVR7G6Y3UBGSJIWT4EZ4CO3T6WVYTMHVSANOY"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (b32:MIZTE===) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "b32:MIZTE==="},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (byte base32:MJ4XIZJAMJQXGZJTGI======) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "byte base32:MJ4XIZJAMJQXGZJTGI======"},
				},
			},
		},
		{
			Name: "DELETED app 5 encoding (byte base64:Ynl0ZSBiYXNlNjQ=) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "byte base64:Ynl0ZSBiYXNlNjQ="},
				},
			},
		},
		{
			Name: "DELETED app 5 illegal encoding (just a plain string) - no params",
			Request: requestInfo{
				Path: "/v2/applications/5/box",
				Params: []param{
					{"name", "just a plain string"},
				},
			},
		},
		{
			Name: "App 1337 non-existing with illegal encoding (just a plain string) - no params",
			Request: requestInfo{
				Path: "/v2/applications/1337/box",
				Params: []param{
					{"name", "just a plain string"},
				},
			},
		},
	},
}

func TestBoxes(t *testing.T) {
	db, dbShutdown := setupIdbAndReturnShutdownFunc(t)
	defer dbShutdown()

	setupLiveBoxes(t, db)

	serverShutdown := setupLiveServerAndReturnShutdownFunc(t, db)
	defer serverShutdown()

	validateOrGenerateFixtures(t, db, boxSeedFixture, "TestBoxes")
}
