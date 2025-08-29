package idb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/idb"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func TestSignatureType(t *testing.T) {
	tests := []struct {
		name     string
		stxn     *sdk.SignedTxn
		expected idb.SigType
		hasError bool
	}{
		{
			name: "Standard signature",
			stxn: &sdk.SignedTxn{
				Sig: sdk.Signature{1, 2, 3}, // non-zero signature
			},
			expected: idb.Sig,
			hasError: false,
		},
		{
			name: "Multisig",
			stxn: &sdk.SignedTxn{
				Msig: sdk.MultisigSig{
					Version:   1,
					Threshold: 2,
				},
			},
			expected: idb.Msig,
			hasError: false,
		},
		{
			name: "Logic sig with standard signature",
			stxn: &sdk.SignedTxn{
				Lsig: sdk.LogicSig{
					Logic: []byte{1, 2, 3},
					Sig:   sdk.Signature{1, 2, 3}, // non-zero signature
				},
			},
			expected: idb.Sig,
			hasError: false,
		},
		{
			name: "Logic sig with multisig",
			stxn: &sdk.SignedTxn{
				Lsig: sdk.LogicSig{
					Logic: []byte{1, 2, 3},
					Msig: sdk.MultisigSig{
						Version:   1,
						Threshold: 2,
					},
				},
			},
			expected: idb.Msig,
			hasError: false,
		},
		{
			name: "Logic sig with LMSig",
			stxn: &sdk.SignedTxn{
				Lsig: sdk.LogicSig{
					Logic: []byte{1, 2, 3},
					LMsig: sdk.MultisigSig{
						Version:   1,
						Threshold: 2,
					},
				},
			},
			expected: idb.Msig,
			hasError: false,
		},
		{
			name: "Pure logic sig",
			stxn: &sdk.SignedTxn{
				Lsig: sdk.LogicSig{
					Logic: []byte{1, 2, 3},
				},
			},
			expected: idb.Lsig,
			hasError: false,
		},
		{
			name: "Logic sig with both Msig and LMSig (LMSig takes precedence)",
			stxn: &sdk.SignedTxn{
				Lsig: sdk.LogicSig{
					Logic: []byte{1, 2, 3},
					Msig: sdk.MultisigSig{
						Version:   1,
						Threshold: 1,
					},
					LMsig: sdk.MultisigSig{
						Version:   1,
						Threshold: 2,
					},
				},
			},
			expected: idb.Msig,
			hasError: false,
		},
		{
			name: "Unsigned transaction",
			stxn: &sdk.SignedTxn{
				// All signature fields are zero
			},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sigType, err := idb.SignatureType(tt.stxn)

			if tt.hasError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to determine the signature type")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, sigType)
			}
		})
	}
}

func TestIsSigTypeValid(t *testing.T) {
	validTypes := []idb.SigType{idb.Sig, idb.Msig, idb.Lsig}
	for _, sigType := range validTypes {
		t.Run(string(sigType), func(t *testing.T) {
			assert.True(t, idb.IsSigTypeValid(sigType))
		})
	}

	invalidTypes := []idb.SigType{"invalid", "unknown", "", "SIG", "MSIG", "LSIG"}
	for _, sigType := range invalidTypes {
		t.Run(string(sigType), func(t *testing.T) {
			assert.False(t, idb.IsSigTypeValid(sigType))
		})
	}
}
