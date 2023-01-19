package main

import (
	"encoding/base64"
	"flag"
	"fmt"

	sdk_types "github.com/algorand/go-algorand-sdk/v2/types"
)

func main() {
	var addrInput string
	flag.StringVar(&addrInput, "addr", "", "base64/algorand address to convert to the other")
	flag.Parse()

	addrBytes, err := base64.StdEncoding.DecodeString(addrInput)
	if err != nil {
		// Failed to base64 decode, try algorand.
		a, err := sdk_types.DecodeAddress(addrInput)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(base64.StdEncoding.EncodeToString(a[:]))
		return
	}
	var addr sdk_types.Address
	copy(addr[:], addrBytes)
	fmt.Println(addr.String())
}
