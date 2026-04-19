//go:build windows
// +build windows

package ebpf

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// WindowsCapabilities describes the eBPF and XDP capabilities on Windows.
type WindowsCapabilities struct {
	// HasXDP indicates if XDP for Windows (xdp.sys) is present
	HasXDP bool
	// XDPVersion is the detected XDP version (e.g., "1.3", "2.0")
	XDPVersion string
	// HaseBPF indicates if eBPF runtime (ebpf-for-windows) is present
	HaseBPF bool
	// eBPFVersion is the detected eBPF version
	eBPFVersion string
	// KernelVersion is the Windows kernel version (e.g., "22623" for Windows 11 23H2)
	KernelVersion string
	// SupportsHVCI indicates if Hypervisor-protected Code Integrity is enabled
	SupportsHVCI bool
	// SupportsNativeMode indicates if native eBPF mode is supported (signed drivers)
	SupportsNativeMode bool
}

var (
	capabilitiesOnce sync.Once
	cachedCapabilities *WindowsCapabilities
)

// DetectCapabilities probes the system for Windows eBPF/XDP capabilities.
// Results are cached after the first call.
func DetectCapabilities() *WindowsCapabilities {
	capabilitiesOnce.Do(func() {
		cachedCapabilities = detectCapabilitiesImpl()
	})
	return cachedCapabilities
}

func detectCapabilitiesImpl() *WindowsCapabilities {
	caps := &WindowsCapabilities{
		HasXDP:             false,
		XDPVersion:         "",
		HaseBPF:            false,
		eBPFVersion:        "",
		KernelVersion:      getWindowsVersion(),
		SupportsHVCI:       false,
		SupportsNativeMode: false,
	}

	// Check for XDP
	caps.HasXDP, caps.XDPVersion = checkXDP()

	// Check for eBPF runtime
	caps.HaseBPF, caps.eBPFVersion = checkeBPF()

	// Check for Hypervisor-protected Code Integrity (HVCI)
	caps.SupportsHVCI = checkHVCI()

	// Native mode requires Windows 11/Server 2022+ with certain kernel features
	caps.SupportsNativeMode = caps.HaseBPF && caps.HasXDP && caps.SupportsHVCI

	return caps
}

// getWindowsVersion returns the Windows kernel build number (e.g., "22621").
func getWindowsVersion() string {
	// Query Windows build version from registry
	// HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows NT\CurrentVersion
	version, err := registryGetString("HKEY_LOCAL_MACHINE", 
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "CurrentBuildNumber")
	if err != nil {
		// Fallback to runtime info
		return runtime.GOOS + "-" + runtime.GOARCH
	}
	return version
}

// registryGetString retrieves a string value from Windows registry
func registryGetString(rootKey string, path string, valueName string) (string, error) {
	// Use powershell to query registry since Go doesn't have direct registry access without CGO
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		fmt.Sprintf(`Get-ItemPropertyValue -Path 'Registry::%s\%s' -Name '%s' -ErrorAction SilentlyContinue`,
			rootKey, path, valueName))
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	
	result := strings.TrimSpace(string(output))
	if result == "" {
		return "", fmt.Errorf("registry key not found")
	}
	
	return result, nil
}

// checkXDP attempts to detect the presence of xdp.sys driver.
func checkXDP() (bool, string) {
	// Query Windows registry or driver store for xdp.sys
	// Primary check: Look in Services registry
	driverState, err := registryGetString("HKEY_LOCAL_MACHINE",
		`SYSTEM\CurrentControlSet\Services\xdp`, "Start")
	if err == nil {
		// Driver is registered. Check if it's enabled (Start=1 means auto/boot, Start=3 means manual)
		state, _ := strconv.Atoi(driverState)
		if state >= 1 && state <= 3 {
			// Try to get version from ImagePath
			imagePath, _ := registryGetString("HKEY_LOCAL_MACHINE",
				`SYSTEM\CurrentControlSet\Services\xdp`, "ImagePath")
			if imagePath != "" {
				// Extract version if available
				return true, getDriverVersion(imagePath)
			}
			return true, "1.x"
		}
	}

	// Secondary check: Look in device drivers
	driverStatus, err := registryGetString("HKEY_LOCAL_MACHINE",
		`SYSTEM\CurrentControlSet\Enum\ROOT`, "Class")
	if err == nil && strings.Contains(driverStatus, "xdp") {
		return true, "1.x"
	}

	// Tertiary check: Run Get-WindowsDriver if available (Windows 10+)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Get-WindowsDriver -Online 2>$null | Where-Object {$_.Driver -like '*xdp*'} | Select-Object -First 1`)
	
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "xdp") {
		return true, "1.x"
	}

	return false, ""
}

// checkeBPF attempts to detect the presence of ebpf-for-windows runtime.
func checkeBPF() (bool, string) {
	// Check for eBPF-for-Windows installation and services
	// Primary: Check for ebpfSvc service
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Get-Service -Name ebpfSvc -ErrorAction SilentlyContinue | Select-Object -First 1`)
	
	output, err := cmd.CombinedOutput()
	if err == nil && (strings.Contains(string(output), "Running") || strings.Contains(string(output), "Stopped")) {
		// eBPF service is installed, try to get version
		version := geteBPFVersion()
		return true, version
	}

	// Secondary: Check registry for eBPF installation
	ebpfPath, _ := registryGetString("HKEY_LOCAL_MACHINE",
		`SOFTWARE\eBPF`, "InstallPath")
	if ebpfPath != "" {
		// eBPF is installed, get version
		version := geteBPFVersion()
		return true, version
	}

	// Tertiary: Check for ebpf.sys driver
	driverState, _ := registryGetString("HKEY_LOCAL_MACHINE",
		`SYSTEM\CurrentControlSet\Services\ebpf`, "Start")
	if driverState != "" {
		state, _ := strconv.Atoi(driverState)
		if state >= 1 && state <= 3 {
			version := geteBPFVersion()
			return true, version
		}
	}

	return false, ""
}

// geteBPFVersion retrieves the eBPF-for-Windows version
func geteBPFVersion() string {
	// Try to query version from registry
	version, err := registryGetString("HKEY_LOCAL_MACHINE",
		`SOFTWARE\eBPF`, "Version")
	if err == nil && version != "" {
		return version
	}

	// Try to get from file version (ebpf.sys)
	sysPath := os.ExpandEnv(`$env:SystemRoot\System32\drivers\ebpf.sys`)
	if fileInfo, err := os.Stat(sysPath); err == nil {
		// Try to extract version from file attributes
		if modTime := fileInfo.ModTime(); !modTime.IsZero() {
			return fmt.Sprintf("0.%d", modTime.Year())
		}
	}

	// Default fallback
	return "0.x"
}

// checkHVCI detects if Hypervisor-protected Code Integrity is enabled.
func checkHVCI() bool {
	// HVCI is enabled via: System Registry
	// HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Control\DeviceGuard
	// Value: RequirePlatformSecurityFeatures = 1 (HVCI enabled)
	
	// Primary check: DeviceGuard registry
	dgValue, err := registryGetString("HKEY_LOCAL_MACHINE",
		`SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity`, "Enabled")
	if err == nil {
		if dgValue == "1" {
			return true
		}
	}

	// Secondary check: Using PowerShell Get-ComputerInfo
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`(Get-ComputerInfo | Select-Object -ExpandProperty DeviceGuardCodeIntegrityStatus) -eq 'Running'`)
	
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "True") {
		return true
	}

	// Tertiary check: Check if secure boot and HVCI capability exist
	cmd2 := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Confirm-SecureBootUEFI -ErrorAction SilentlyContinue`)
	
	output2, err2 := cmd2.CombinedOutput()
	if err2 == nil && (strings.Contains(string(output2), "True") || strings.Contains(string(output2), "True")) {
		// Secure Boot is possible indicator of capability
		// Further check via WMI
		cmd3 := exec.Command("wmic", "os", "get", "dataexecutionprevention_available", "/format:list")
		if output3, err3 := cmd3.CombinedOutput(); err3 == nil && strings.Contains(string(output3), "TRUE") {
			return true
		}
	}

	return false
}

// getDriverVersion extracts version string from driver path
func getDriverVersion(driverPath string) string {
	// Try to get file version using PowerShell
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		fmt.Sprintf(`[System.Diagnostics.FileVersionInfo]::GetVersionInfo('%s').FileVersion`, driverPath))
	
	output, err := cmd.CombinedOutput()
	if err == nil {
		version := strings.TrimSpace(string(output))
		if version != "" && version != "<nil>" {
			return version
		}
	}

	return "1.x" // Fallback
}

// IsXDPSupported returns true if the system supports XDP for packet processing.
func IsXDPSupported() bool {
	caps := DetectCapabilities()
	return caps.HasXDP && caps.KernelVersion != ""
}

// IseBPFSupported returns true if the system supports eBPF programs.
func IseBPFSupported() bool {
	caps := DetectCapabilities()
	return caps.HaseBPF && caps.KernelVersion != ""
}

// IsAccelerationAvailable returns true if any acceleration path (XDP or eBPF) is available.
func IsAccelerationAvailable() bool {
	caps := DetectCapabilities()
	return caps.HasXDP || caps.HaseBPF
}

// GetFeatureSummary returns a human-readable summary of eBPF/XDP support.
func GetFeatureSummary() string {
	caps := DetectCapabilities()
	
	summary := "Windows eBPF/XDP Support:\n"
	summary += "  XDP: " + boolStr(caps.HasXDP)
	if caps.HasXDP {
		summary += " (v" + caps.XDPVersion + ")"
	}
	summary += "\n"
	summary += "  eBPF: " + boolStr(caps.HaseBPF)
	if caps.HaseBPF {
		summary += " (v" + caps.eBPFVersion + ")"
	}
	summary += "\n"
	summary += "  HVCI: " + boolStr(caps.SupportsHVCI) + "\n"
	summary += "  Native Mode: " + boolStr(caps.SupportsNativeMode) + "\n"
	
	return summary
}

func boolStr(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}
