/*
Package abi provides an implementation of the Algorand ARC-4 ABI type system.

See https://arc.algorand.foundation/ARCs/arc-0004 for the corresponding specification.


Basic Operations

This package can parse ABI type names using the `abi.TypeOf()` function.

`abi.TypeOf()` returns an `abi.Type` struct. The `abi.Type` struct's `Encode` and `Decode` methods
can convert between Go values and encoded ABI byte strings.
*/
package abi
