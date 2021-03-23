package types

import (
	"fmt"

	"github.com/algorand/go-algorand-sdk/encoding/json"
)

// protocols.json from code run in go-algorand:
// goal protocols > protocols.json
//go:generate go run ../cmd/texttosource/main.go types protocols.json

var protocols map[string]ConsensusParams

func ensureProtos() (err error) {
	if protocols != nil {
		return nil
	}
	protos := make(map[string]ConsensusParams, 30)
	// Load text from protocols.json as compiled-in.
	err = json.Decode([]byte(protocols_json), &protos)
	if err != nil {
		return fmt.Errorf("proto decode, %v", err)
	}
	protocols = protos
	return nil
}

type UnknownProtocol struct {
	BadVersion string
}

// implemnent error interface
func (up UnknownProtocol) Error() string {
	return fmt.Sprintf("Unknown protocol: %s", up.BadVersion)
}

func Protocol(version string) (proto ConsensusParams, err error) {
	err = ensureProtos()
	if err != nil {
		return
	}
	var ok bool
	proto, ok = protocols[version]
	if !ok {
		err = &UnknownProtocol{version}
	}
	return
}

func ForeachProtocol(f func(version string, proto ConsensusParams) error) (err error) {
	err = ensureProtos()
	if err != nil {
		return
	}
	for version, proto := range protocols {
		err = f(version, proto)
		if err != nil {
			return
		}
	}
	return nil
}
