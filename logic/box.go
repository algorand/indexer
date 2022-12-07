package logic

import (
	"encoding/binary"
	"fmt"

	"github.com/algorand/go-algorand-sdk/types"
)

const boxPrefix = "bx:"
const boxPrefixLength = len(boxPrefix)
const boxNameIndex = boxPrefixLength + 8 // len("bx:") + 8 (appIdx, big-endian)

// MakeBoxKey creates the key that a box named `name` under app `appIdx` should use.
func MakeBoxKey(appIdx types.AppIndex, name string) string {
	/* This format is chosen so that a simple indexing scheme on the key would
	   allow for quick lookups of all the boxes of a certain app, or even all
	   the boxes of a certain app with a certain prefix.

	   The "bx:" prefix is so that the kvstore might be usable for things
	   besides boxes.
	*/
	key := make([]byte, boxNameIndex+len(name))
	copy(key, boxPrefix)
	binary.BigEndian.PutUint64(key[boxPrefixLength:], uint64(appIdx))
	copy(key[boxNameIndex:], name)
	return string(key)
}

// SplitBoxKey extracts an appid and box name from a string that was created by MakeBoxKey()
func SplitBoxKey(key string) (types.AppIndex, string, error) {
	if len(key) < boxNameIndex {
		return 0, "", fmt.Errorf("SplitBoxKey() cannot extract AppIndex as key (%s) too short (length=%d)", key, len(key))
	}
	if key[:boxPrefixLength] != boxPrefix {
		return 0, "", fmt.Errorf("SplitBoxKey() illegal app box prefix in key (%s). Expected prefix '%s'", key, boxPrefix)
	}
	keyBytes := []byte(key)
	app := types.AppIndex(binary.BigEndian.Uint64(keyBytes[boxPrefixLength:boxNameIndex]))
	return app, key[boxNameIndex:], nil
}
