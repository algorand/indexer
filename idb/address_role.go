package idb

// AddressRole is a dedicated type for the address role.
type AddressRole uint64

// All possible address roles.
const (
	AddressRoleSender           AddressRole = 0x01
	AddressRoleReceiver         AddressRole = 0x02
	AddressRoleCloseRemainderTo AddressRole = 0x04
	AddressRoleAssetSender      AddressRole = 0x08
	AddressRoleAssetReceiver    AddressRole = 0x10
	AddressRoleAssetCloseTo     AddressRole = 0x20
	AddressRoleFreeze           AddressRole = 0x40
)
