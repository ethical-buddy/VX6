package onion

import (
	"context"
	"testing"
)

func BenchmarkParseXDPStatusJSONActive(b *testing.B) {
	payload := []byte(`[{"ifname":"eth0","xdp":{"mode":"xdpgeneric","attached":true,"prog":{"id":41,"name":"xdp_onion_relay","tag":"abc123"}}}]`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		status, err := parseXDPStatusJSON(payload, "eth0")
		if err != nil {
			b.Fatal(err)
		}
		status = finalizeXDPStatus(status)
		if !status.VX6Active {
			b.Fatalf("expected VX6 active status, got %+v", status)
		}
	}
}

func BenchmarkParseXDPStatusTextActive(b *testing.B) {
	payload := []byte("2: eth0: <BROADCAST> mtu 1500 xdpgeneric prog/xdp id 91 name xdp_onion_relay")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		status := parseXDPStatusText(payload, "eth0")
		if !status.VX6Active {
			b.Fatalf("expected VX6 active status, got %+v", status)
		}
	}
}

func BenchmarkXDPManagerStatusFakeRunner(b *testing.B) {
	manager := &XDPManager{
		runner: fakeRunner{
			outputs: map[string]fakeResult{
				"ip -j -d link show dev eth0": {output: []byte(`[{"ifname":"eth0","xdp":{"mode":"xdpgeneric","prog":{"id":17,"name":"xdp_onion_relay"}}}]`)},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		status, err := manager.Status(context.Background(), "eth0")
		if err != nil {
			b.Fatal(err)
		}
		if !status.VX6Active {
			b.Fatalf("expected VX6 active status, got %+v", status)
		}
	}
}
