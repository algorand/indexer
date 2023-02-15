package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/indexer/conduit/plugins/processors/filterprocessor/gen/internal"
)

func TestIgnoreTags(t *testing.T) {
	testStruct := struct {
		One   string `codec:"one"`
		Two   string `codec:"two"`
		Three string `codec:"three"`
	}{}

	ignoreTags = map[string]bool{
		"one":   true,
		"three": true,
	}
	f, errs := getFields(testStruct)
	require.Len(t, errs, 0)
	assert.Len(t, f, 1)
	// Make sure it found the field which wasn't ignored.
	assert.Equal(t, internal.StructField{
		TagPath:    "two",
		FieldPath:  "Two",
		CastPrefix: "",
		CastPost:   "",
	}, f["two"])
}

func TestIgnoreMiddleTags(t *testing.T) {
	testStruct := struct {
		Parent struct {
			Child string `codec:"child"`
		} `codec:"parent"`
	}{}
	f, errs := getFields(testStruct)
	require.Len(t, errs, 0)
	assert.Len(t, f, 1)
	// Make sure parent is not returned on its own.
	assert.Equal(t, internal.StructField{
		TagPath:    "parent.child",
		FieldPath:  "Parent.Child",
		CastPrefix: "",
		CastPost:   "",
	}, f["parent.child"])
}

func TestNoCast(t *testing.T) {
	testStruct := struct {
		One string `codec:"one"`
	}{}
	f, errs := getFields(testStruct)
	require.Len(t, errs, 0)
	assert.Len(t, f, 1)
	// Make sure it found the field which wasn't ignored.
	assert.Equal(t, internal.StructField{
		TagPath:    "one",
		FieldPath:  "One",
		CastPrefix: "", // no cast
		CastPost:   "", // no cast
	}, f["one"])
}

func TestSimpleCast(t *testing.T) {
	testStruct := struct {
		One int32 `codec:"one"`
	}{}
	f, errs := getFields(testStruct)
	require.Len(t, errs, 0)
	assert.Len(t, f, 1)
	// Make sure it found the field which wasn't ignored.
	assert.Equal(t, internal.StructField{
		TagPath:    "one",
		FieldPath:  "One",
		CastPrefix: "int64(", // simple cast
		CastPost:   ")",      // simple cast
	}, f["one"])
}

func TestComplexCast(t *testing.T) {
	testcases := []struct {
		name       string
		testStruct interface{}
		castPrefix string
		castPost   string
	}{
		{
			name: "bytes",
			testStruct: struct {
				One [32]byte `codec:"one"`
			}{},
			castPrefix: "base64.StdEncoding.EncodeToString(",
			castPost:   "[:])",
		},
		{
			name: "address",
			testStruct: struct {
				One sdk.Address `codec:"one"`
			}{},
			castPrefix: "",
			castPost:   ".String()",
		},
		{
			name: "bool",
			testStruct: struct {
				One bool `codec:"one"`
			}{},
			castPrefix: "fmt.Sprintf(\"%t\", ",
			castPost:   ")",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, errs := getFields(tc.testStruct)
			require.Len(t, errs, 0)
			assert.Len(t, f, 1)
			for _, v := range f {
				assert.Equal(t, tc.castPrefix, v.CastPrefix)
				assert.Equal(t, tc.castPost, v.CastPost)
			}
		})
	}
}
