//go:build !windows

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

func registerReloadSignal(sigCh chan os.Signal) {
	signal.Notify(sigCh, syscall.SIGHUP)
}

func processExists(pid int) error {
	return syscall.Kill(pid, 0)
}

func sendReloadSignal(pid int) error {
	return syscall.Kill(pid, syscall.SIGHUP)
}
