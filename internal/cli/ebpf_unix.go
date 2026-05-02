//go:build !windows

package cli

// PrintEBPFPlatformNote is a stub for non-Windows platforms
// On Linux, eBPF is handled by the onion package
func PrintEBPFPlatformNote() {
	// No platform-specific note needed on Linux
}
