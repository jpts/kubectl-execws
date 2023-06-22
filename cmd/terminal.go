package cmd

import (
	"fmt"

	"github.com/moby/term"
)

type TerminalState struct {
	Size        TerminalSize
	StdInFd     uintptr
	StdOutFd    uintptr
	StateBlob   *term.State
	Initialised bool
	IsRaw       bool
}

type TerminalSize struct {
	Width  int `json:"Width"`
	Height int `json:"Height"`
}

func updateSize(state *TerminalState) (bool, error) {
	storedSize := state.Size
	fd := state.StdOutFd

	ws, err := term.GetWinsize(fd)
	if err != nil {
		return false, fmt.Errorf("Failed to get terminal size: %w", err)
	}
	newSize := TerminalSize{
		Height: int(ws.Height),
		Width:  int(ws.Width),
	}

	if newSize != storedSize {
		state.Size = newSize
		return true, nil
	}

	return false, nil
}
