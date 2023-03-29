//go:build linux || darwin

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

func registerResizeSignal() chan os.Signal {
	resizeNotify := make(chan os.Signal, 1)
	signal.Notify(resizeNotify, syscall.SIGWINCH)
	return resizeNotify
}

func waitForResizeChange(sig chan os.Signal) {
	_ = <-sig
}
