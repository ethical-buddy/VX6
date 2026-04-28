package onion

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

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

func TestParseXDPStatusJSONAttachedVX6Program(t *testing.T) {
	t.Parallel()

	status, err := parseXDPStatusJSON([]byte(`[
  {
    "ifname": "eth0",
    "xdp": {
      "mode": "xdpgeneric",
      "attached": true,
      "prog": {
        "id": 41,
        "name": "xdp_onion_relay",
        "tag": "abc123"
      }
    }
  }
]`), "eth0")
	if err != nil {
		t.Fatalf("parse status: %v", err)
	}
	status = finalizeXDPStatus(status)
	if !status.Attached || !status.VX6Active {
		t.Fatalf("expected VX6 XDP to be active, got %+v", status)
	}
	if status.Mode != "generic" || status.ProgramID != 41 || status.ProgramName != xdpProgramName {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
}

func TestParseXDPStatusJSONAttachedDifferentProgram(t *testing.T) {
	t.Parallel()

	status, err := parseXDPStatusJSON([]byte(`[
  {
    "ifname": "eth0",
    "xdp": {
      "mode": "native",
      "prog": {
        "id": 77,
        "name": "some_other_prog"
      }
    }
  }
]`), "eth0")
	if err != nil {
		t.Fatalf("parse status: %v", err)
	}
	status = finalizeXDPStatus(status)
	if !status.Attached {
		t.Fatalf("expected attached status, got %+v", status)
	}
	if status.VX6Active {
		t.Fatalf("expected non-VX6 XDP program to remain inactive for VX6, got %+v", status)
	}
}

func TestParseXDPStatusTextFallback(t *testing.T) {
	t.Parallel()

	status := parseXDPStatusText([]byte("2: eth0: <BROADCAST> mtu 1500 xdpgeneric prog/xdp id 91 name xdp_onion_relay"), "eth0")
	if !status.Attached || !status.VX6Active {
		t.Fatalf("expected parsed fallback status to be active, got %+v", status)
	}
	if status.Mode != "generic" || status.ProgramID != 91 {
		t.Fatalf("unexpected fallback status: %+v", status)
	}
}

func TestXDPManagerStatusUsesLiveRunner(t *testing.T) {
	t.Parallel()

	manager := &XDPManager{
		runner: fakeRunner{
			outputs: map[string]fakeResult{
				"ip -j -d link show dev eth0": {output: []byte(`[{"ifname":"eth0","xdp":{"mode":"xdpgeneric","prog":{"id":17,"name":"xdp_onion_relay"}}}]`)},
			},
		},
	}

	status, err := manager.Status(context.Background(), "eth0")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Attached || !status.VX6Active || status.ProgramID != 17 {
		t.Fatalf("unexpected status result: %+v", status)
	}
}

func TestXDPManagerAttachFallsBackToGeneric(t *testing.T) {
	t.Parallel()

	manager := &XDPManager{
		runner: fakeRunner{
			outputs: map[string]fakeResult{
				"ip link set dev eth0 xdp obj __TEMP__ sec xdp":        {err: fmt.Errorf("native attach failed")},
				"ip link set dev eth0 xdpgeneric obj __TEMP__ sec xdp": {output: []byte("ok")},
				"ip -j -d link show dev eth0":                          {output: []byte(`[{"ifname":"eth0","xdp":{"mode":"xdpgeneric","prog":{"id":12,"name":"xdp_onion_relay"}}}]`)},
			},
			normalizeTempObject: true,
		},
	}

	status, err := manager.Attach(context.Background(), "eth0")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if !status.Attached || !status.VX6Active || status.Mode != "generic" {
		t.Fatalf("unexpected attach status: %+v", status)
	}
}

func TestXDPManagerDetachReportsClearedState(t *testing.T) {
	t.Parallel()

	manager := &XDPManager{
		runner: fakeRunner{
			outputs: map[string]fakeResult{
				"ip link set dev eth0 xdp off": {output: []byte("ok")},
				"ip -j -d link show dev eth0":  {output: []byte(`[{"ifname":"eth0"}]`)},
			},
		},
	}

	status, err := manager.Detach(context.Background(), "eth0")
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if status.Attached || status.VX6Active {
		t.Fatalf("expected detached status, got %+v", status)
	}
}

type fakeResult struct {
	output []byte
	err    error
}

type fakeRunner struct {
	outputs             map[string]fakeResult
	normalizeTempObject bool
}

func (f fakeRunner) CombinedOutput(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if f.normalizeTempObject {
		key = normalizeTempObjectArg(key)
	}
	result, ok := f.outputs[key]
	if !ok {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
	return result.output, result.err
}

func normalizeTempObjectArg(command string) string {
	parts := strings.Fields(command)
	for i, part := range parts {
		if strings.HasPrefix(part, "/") && strings.HasSuffix(part, ".o") {
			parts[i] = "__TEMP__"
		}
	}
	return strings.Join(parts, " ")
}
