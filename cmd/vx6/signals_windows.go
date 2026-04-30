//go:build windows

package main

import "os"

func notifySignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
