package cmd

import "github.com/moby/term"

func updateSize(state *TerminalState) (bool, error) {
	storedSize := state.Size

	ws, err := term.GetWinsize(state.Fd)
	if err != nil {
		return false, err
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
