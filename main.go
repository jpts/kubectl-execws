package main

import (
	"os"
	"path/filepath"

	"github.com/jpts/kubectl-execws/cmd"
)

func main() {
	name := filepath.Base(os.Args[0])
	switch name {
	case "kubectl_complete-execws":
		cmd.Complete()
	default:
		cmd.Execute()
	}
}
