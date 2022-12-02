package basics

import (
	"encoding/binary"

	"github.com/algorand/indexer/crypto"
	"github.com/algorand/indexer/protocol"
)

// AppIndex is the unique integer index of an application that can be used to
// look up the creator of the application, whose balance record contains the
// AppParams
type AppIndex uint64

// ToBeHashed implements crypto.Hashable
func (app AppIndex) ToBeHashed() (protocol.HashID, []byte) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(app))
	return protocol.AppIndex, buf
}

// Address yields the "app address" of the app
func (app AppIndex) Address() Address {
	return Address(crypto.HashObj(app))
}
