//go:build windows

package cmd

import (
	"os"
	"time"
)

func registerResizeSignal() chan os.Signal {
	return nil
}

func waitForResizeChange(_ chan os.Signal) {
	time.Sleep(250 * time.Millisecond)
}
