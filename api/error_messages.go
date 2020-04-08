package api

const (
	errInvalidRoundMinMax   = "cannot specify round and min-round/max-round"
	errUnableToParseAddress = "unable to parse address"
	errUnknownAddressRole   = "unknown address role [valid roles: sender, receiver, freeze-target]"
	errUnknownSigType       = "unknown sig-type [valid types: sig, lsig, msig]"
	errUnknownTxType        = "unknown tx-type [valid types: pay, keyreg, acfg, axfer, afrz]"
)
