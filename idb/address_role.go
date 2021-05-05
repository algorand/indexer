package idb

type AddressRole uint64

const (
	AddressRoleSender           AddressRole = 0x01
	AddressRoleReceiver         AddressRole = 0x02
	AddressRoleCloseRemainderTo AddressRole = 0x04
	AddressRoleAssetSender      AddressRole = 0x08
	AddressRoleAssetReceiver    AddressRole = 0x10
	AddressRoleAssetCloseTo     AddressRole = 0x20
	AddressRoleFreeze           AddressRole = 0x40
)
