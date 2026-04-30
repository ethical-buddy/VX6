//go:build !windows

package main

import (
	"os"
	"syscall"
)

func notifySignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
