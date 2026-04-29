package onion

import "testing"

func TestEmbeddedRelayBytecode(t *testing.T) {
	t.Parallel()

	if !IsEBPFAvailable() {
		t.Fatal("expected embedded eBPF relay bytecode")
	}
	if len(OnionRelayBytecode) < 4 {
		t.Fatal("embedded eBPF bytecode is unexpectedly short")
	}
	if string(OnionRelayBytecode[:4]) != "\x7fELF" {
		t.Fatalf("embedded eBPF object is not an ELF file")
	}
}
