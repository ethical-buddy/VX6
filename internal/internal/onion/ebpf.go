package onion

import (
	_ "embed"
)

//go:embed onion_relay.o
var OnionRelayBytecode []byte

// IsEBPFAvailable checks if the current binary has embedded kernel bytecode.
func IsEBPFAvailable() bool {
	return len(OnionRelayBytecode) > 0
}
