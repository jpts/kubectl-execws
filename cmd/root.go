package cmd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var (
	kconfig       string
	tty           bool
	stdinFlag     bool
	quiet         bool
	container     string
	namespace     string
	loglevel      int
	noSanityCheck bool
	noTLSVerify   bool
)

var rootCmd = &cobra.Command{
	Use:           "execws <pod name> [--kubeconfig <path>] [-n namespace] [-it] [-c container] <cmd>",
	Short:         "kubectl exec over WebSockets",
	Long:          `A replacement for "kubectl exec" that works over WebSocket connections.`,
	Args:          cobra.MinimumNArgs(2),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return errors.New("Too many arguments")
		}

		s := &cliSession{
			opts: Options{
				Command:       command,
				Container:     container,
				Kconfig:       kconfig,
				Namespace:     namespace,
				Object:        object,
				Pod:           pod,
				Stdin:         stdinFlag,
				TTY:           tty,
				noSanityCheck: noSanityCheck,
				noTLSVerify:   noTLSVerify,
			},
		}

		err := s.prepConfig()
		if err != nil {
			return err
		}

		req, err := s.prepExec()
		if err != nil {
			return err
		}

		return s.doExec(req)
	},
}

func Execute() {
	klog.InitFlags(nil)
	flag.Set("v", fmt.Sprint(loglevel))
	flag.Set("stderrthreshold", fmt.Sprint(loglevel))
	//flag.Parse()

	err := rootCmd.Execute()
	if err != nil {
		klog.Exit(err)
	}
	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kconfig, "kubeconfig", "", "kubeconfig file (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Override \"default\" namespace")
	rootCmd.PersistentFlags().IntVarP(&loglevel, "loglevel", "v", 2, "Set loglevel")
	rootCmd.PersistentFlags().BoolVarP(&noTLSVerify, "skip-tls-verify", "k", false, "Don't perform TLS certificate verifiation")

	rootCmd.Flags().BoolVarP(&tty, "tty", "t", false, "Stdin is a TTY")
	rootCmd.Flags().BoolVarP(&stdinFlag, "stdin", "i", false, "Pass stdin to container")
	rootCmd.Flags().StringVarP(&container, "container", "c", "", "Container name")
	rootCmd.Flags().BoolVar(&noSanityCheck, "no-sanity-check", false, "Don't check pod exists before exec")
}
