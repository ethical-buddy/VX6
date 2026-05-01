//go:build windows

package cli

import (
	"fmt"
)

// PrintEBPFPlatformNote prints Windows-specific eBPF information
func PrintEBPFPlatformNote() {
	fmt.Println("ebpf_support\tfalse")
	fmt.Println("platform_note\teBPF/XDP kernel acceleration is a Linux-only feature")
	fmt.Println("fallback_mode\tVX6 uses the standard user-space TCP relay path on Windows")
}
