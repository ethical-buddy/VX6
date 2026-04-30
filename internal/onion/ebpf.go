package onion

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

//go:embed onion_relay.o
var OnionRelayBytecode []byte

const xdpProgramSection = "xdp"
const xdpProgramName = "xdp_onion_relay"

type XDPStatus struct {
	Interface            string
	Attached             bool
	VX6Active            bool
	Mode                 string
	ProgramName          string
	ProgramID            int
	ProgramTag           string
	EmbeddedBytecode     bool
	BytecodeSize         int
	CompatibilityWarning string
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type systemRunner struct{}

func (systemRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type XDPManager struct {
	runner commandRunner
}

// IsEBPFAvailable checks if the current binary has embedded kernel bytecode.
func IsEBPFAvailable() bool {
	return len(OnionRelayBytecode) > 0
}

func NewXDPManager() *XDPManager {
	return &XDPManager{runner: systemRunner{}}
}

func (m *XDPManager) Status(ctx context.Context, iface string) (XDPStatus, error) {
	status := baseXDPStatus(iface)
	if !xdpSupported() {
		return status, fmt.Errorf("xdp/eBPF management is only supported on Linux")
	}
	if strings.TrimSpace(iface) == "" {
		return status, fmt.Errorf("interface is required")
	}
	out, err := m.runner.CombinedOutput(ctx, "ip", "-j", "-d", "link", "show", "dev", iface)
	if err != nil {
		return status, fmt.Errorf("inspect XDP status for %s: %w", iface, err)
	}
	parsed, err := parseXDPStatusJSON(out, iface)
	if err != nil {
		textOut, textErr := m.runner.CombinedOutput(ctx, "ip", "-d", "link", "show", "dev", iface)
		if textErr != nil {
			return status, fmt.Errorf("decode XDP status for %s: %w", iface, err)
		}
		return parseXDPStatusText(textOut, iface), nil
	}
	return finalizeXDPStatus(parsed), nil
}

func (m *XDPManager) Attach(ctx context.Context, iface string) (XDPStatus, error) {
	if !xdpSupported() {
		return baseXDPStatus(iface), fmt.Errorf("xdp/eBPF attach is only supported on Linux")
	}
	if strings.TrimSpace(iface) == "" {
		return baseXDPStatus(""), fmt.Errorf("interface is required")
	}
	if !IsEBPFAvailable() {
		return baseXDPStatus(iface), fmt.Errorf("embedded eBPF bytecode is not available")
	}

	tmpDir, err := os.MkdirTemp("", "vx6-ebpf-*")
	if err != nil {
		return baseXDPStatus(iface), fmt.Errorf("create temp eBPF directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	objPath := filepath.Join(tmpDir, "onion_relay.o")
	if err := os.WriteFile(objPath, OnionRelayBytecode, 0o600); err != nil {
		return baseXDPStatus(iface), fmt.Errorf("write eBPF object: %w", err)
	}

	type attachMode struct {
		name string
		args []string
	}
	modes := []attachMode{
		{name: "native", args: []string{"link", "set", "dev", iface, "xdp", "obj", objPath, "sec", xdpProgramSection}},
		{name: "generic", args: []string{"link", "set", "dev", iface, "xdpgeneric", "obj", objPath, "sec", xdpProgramSection}},
	}

	failures := make([]string, 0, len(modes))
	for _, mode := range modes {
		if _, err := m.runner.CombinedOutput(ctx, "ip", mode.args...); err == nil {
			status, statusErr := m.Status(ctx, iface)
			if statusErr != nil {
				return baseXDPStatus(iface), statusErr
			}
			if !status.Attached {
				return status, fmt.Errorf("XDP attach command succeeded but no XDP program is attached on %s", iface)
			}
			return status, nil
		} else {
			failures = append(failures, fmt.Sprintf("%s: %v", mode.name, err))
		}
	}

	return baseXDPStatus(iface), fmt.Errorf("attach XDP program failed: %s", strings.Join(failures, " | "))
}

func (m *XDPManager) Detach(ctx context.Context, iface string) (XDPStatus, error) {
	if !xdpSupported() {
		return baseXDPStatus(iface), fmt.Errorf("xdp/eBPF detach is only supported on Linux")
	}
	if strings.TrimSpace(iface) == "" {
		return baseXDPStatus(""), fmt.Errorf("interface is required")
	}
	if _, err := m.runner.CombinedOutput(ctx, "ip", "link", "set", "dev", iface, "xdp", "off"); err != nil {
		return baseXDPStatus(iface), fmt.Errorf("detach XDP program from %s: %w", iface, err)
	}
	status, err := m.Status(ctx, iface)
	if err != nil {
		return baseXDPStatus(iface), err
	}
	return status, nil
}

func baseXDPStatus(iface string) XDPStatus {
	return XDPStatus{
		Interface:            iface,
		EmbeddedBytecode:     IsEBPFAvailable(),
		BytecodeSize:         len(OnionRelayBytecode),
		CompatibilityWarning: xdpCompatibilityWarning(),
	}
}

func finalizeXDPStatus(status XDPStatus) XDPStatus {
	status.EmbeddedBytecode = IsEBPFAvailable()
	status.BytecodeSize = len(OnionRelayBytecode)
	status.CompatibilityWarning = xdpCompatibilityWarning()
	status.VX6Active = status.Attached && status.ProgramName == xdpProgramName
	return status
}

func xdpCompatibilityWarning() string {
	if !xdpSupported() {
		return "XDP/eBPF status and attach commands are Linux-only; this VX6 build uses the normal user-space TCP path on this platform"
	}
	return "embedded XDP program targets the legacy VX6 onion header and is not yet the active fast path for the current encrypted relay data path"
}

func xdpSupported() bool {
	return runtime.GOOS == "linux"
}

func parseXDPStatusJSON(data []byte, iface string) (XDPStatus, error) {
	status := baseXDPStatus(iface)
	var links []map[string]any
	if err := json.Unmarshal(data, &links); err != nil {
		return status, err
	}
	if len(links) == 0 {
		return status, fmt.Errorf("no link data returned")
	}
	link := links[0]
	if xdpValue, ok := link["xdp"]; ok {
		xdpMap, ok := xdpValue.(map[string]any)
		if !ok {
			return status, nil
		}
		if mode, ok := xdpMap["mode"].(string); ok {
			status.Mode = normalizeXDPMode(mode)
		}
		if attached, ok := xdpMap["attached"].(bool); ok {
			status.Attached = attached
		}
		if progValue, ok := xdpMap["prog"]; ok {
			if progMap, ok := progValue.(map[string]any); ok {
				if name, ok := progMap["name"].(string); ok {
					status.ProgramName = name
				}
				if tag, ok := progMap["tag"].(string); ok {
					status.ProgramTag = tag
				}
				status.ProgramID = parseJSONInt(progMap["id"])
			}
		}
	}

	if status.Mode != "" || status.ProgramID > 0 || status.ProgramName != "" {
		status.Attached = true
	}
	return status, nil
}

func parseJSONInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

func normalizeXDPMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "xdp", "drv", "native":
		return "native"
	case "xdpgeneric", "skb", "generic":
		return "generic"
	case "xdpoffload", "hw", "offload":
		return "offload"
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

var (
	textXDPProgRE = regexp.MustCompile(`prog/xdp id ([0-9]+)`)
	textXDPNameRE = regexp.MustCompile(`name ([^ ]+)`)
)

func parseXDPStatusText(data []byte, iface string) XDPStatus {
	status := baseXDPStatus(iface)
	text := string(data)
	switch {
	case strings.Contains(text, "xdpgeneric"):
		status.Mode = "generic"
	case strings.Contains(text, "xdpdrv") || strings.Contains(text, "xdp "):
		status.Mode = "native"
	case strings.Contains(text, "xdpoffload"):
		status.Mode = "offload"
	}

	if match := textXDPProgRE.FindStringSubmatch(text); len(match) == 2 {
		status.Attached = true
		status.ProgramID, _ = strconv.Atoi(match[1])
	}
	if match := textXDPNameRE.FindStringSubmatch(text); len(match) == 2 {
		status.ProgramName = match[1]
	}
	if status.Mode != "" {
		status.Attached = true
	}
	return finalizeXDPStatus(status)
}
