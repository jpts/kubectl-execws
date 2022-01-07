package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	kconfig   string
	tty       bool
	stdinFlag bool
	quiet     bool
	container string
	namespace string
)

var rootCmd = &cobra.Command{
        Use:   "execws <pod name> [--kubeconfig] [-n namespace] [-it] [-c container] <cmd>",
	Short: "kubectl exec over WebSockets",
	Long:  `A replacement for "kubectl exec" that works over WebSocket connections.`,
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		var object, pod string
		var command []string
		if len(args) == 3 {
			object = args[0]
			pod = args[1]
			command = args[2:]
		} else if strings.Contains(args[0], "/") {
			parts := strings.Split(args[0], "/")
			object = parts[0]
			pod = parts[1]
			command = args[1:]
		} else if len(args) == 2 {
			object = "pod"
			pod = args[0]
			command = args[1:]
		} else {
			fmt.Println("bad input")
                        os.Exit(1)
		}

		opts := &Options{
			Command:   command,
			Container: container,
			Kconfig:   kconfig,
			Namespace: namespace,
			Object:    object,
			Pod:       pod,
			Stdin:     stdinFlag,
			TTY:       tty,
		}

		req, _ := prepExec(opts)

                err := doExec(req)
		if err != nil {
			fmt.Println(err.Error())
                        os.Exit(1)
		}

	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kconfig, "kubeconfig", "", "kubeconfig file (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Override \"default\" namespace")
	rootCmd.Flags().BoolVarP(&tty, "tty", "t", false, "Stdin is a TTY")
	rootCmd.Flags().BoolVarP(&stdinFlag, "stdin", "i", false, "Pass stdin to container")
	rootCmd.Flags().StringVarP(&container, "container", "c", "", "Container name")
	//rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "")
	//rootCmd.Flags().BoolVarP(&verb, "verbose", "v", false, "")

}
