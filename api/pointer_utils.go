package api

import (
	"time"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/go-algorand/data/basics"
)

////////////////////////////////
// Safe dereference wrappers. //
////////////////////////////////
func uintOrDefault(x *uint64) uint64 {
	if x != nil {
		return *x
	}
	return 0
}

func uintOrDefaultValue(x *uint64, value uint64) uint64 {
	if x != nil {
		return *x
	}
	return value
}

func strOrDefault(str *string) string {
	if str != nil {
		return *str
	}
	return ""
}

func boolOrDefault(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

////////////////////////////
// Safe pointer wrappers. //
////////////////////////////
func uint64Ptr(x uint64) *uint64 {
	return &x
}

func uint64PtrOrNil(x uint64) *uint64 {
	if x == 0 {
		return nil
	}
	return &x
}

func byteSlicePtr(x []byte) *[]byte {
	if len(x) == 0 {
		return nil
	}

	xx := make([]byte, len(x))
	copy(xx, x)
	return &xx
}

func byteSliceOmitZeroPtr(x []byte) *[]byte {
	// Don't return if it's all zero.
	for _, v := range x {
		if v != 0 {
			xx := make([]byte, len(x))
			copy(xx, x)
			return &xx
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

func addrPtr(x sdk.Address) *string {
	if x.IsZero() {
		return nil
	}
	out := new(string)
	*out = x.String()
	return out
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

func strArrayPtr(x []string) *[]string {
	if len(x) == 0 {
		return nil
	}
	return &x
}

func addrSlice(x basics.Address) []byte {
	xx := new(basics.Address)
	*xx = x
	return xx[:]
}
