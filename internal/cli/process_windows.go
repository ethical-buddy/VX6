//go:build windows

package cli

import (
	"errors"
	"os"
	"syscall"
)

func registerReloadSignal(_ chan os.Signal) {}

func processExists(pid int) error {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return nil
		}
		return err
	}
	defer syscall.CloseHandle(h)

	state, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return err
	}
	if state != syscall.WAIT_TIMEOUT {
		return errors.New("process is not running")
	}
	return nil
}

func sendReloadSignal(_ int) error {
	return errors.New("signal-based reload is not supported on windows; retry when node control channel is available")
}
