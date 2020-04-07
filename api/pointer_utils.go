package api

import (
	"time"

	"github.com/algorand/go-algorand-sdk/types"
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

func uintOrDefaultMod(x *uint64, modifier int64) uint64 {
	if x != nil {
		val := int64(*x) + modifier
		if val < 0 {
			return 0
		}
		return uint64(val)
	}
	return 0
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

func uint64PtrOrNil(x uint64) *uint64 {
	if x == 0 {
		return nil
	}
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

func addrPtr(x types.Address) *string {
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
