package cmd

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var (
	releaseVersion   string
	kconfig          string
	tty              bool
	stdinFlag        bool
	quiet            bool
	container        string
	namespace        string
	loglevel         int
	noSanityCheck    bool
	noTLSVerify      bool
	directExec       bool
	directExecNodeIp string
	base64SubProto   bool
)

var rootCmd = &cobra.Command{
	Use:           "execws <pod name> [options] -- <cmd>",
	Short:         "kubectl exec over WebSockets",
	Long:          `A replacement for "kubectl exec" that works over WebSocket connections.`,
	Args:          cobra.MinimumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var object, pod string
		var command []string

		if strings.Contains(args[0], "/") {
			parts := strings.Split(args[0], "/")
			object = parts[0]
			pod = parts[1]
			command = args[1:]
		} else {
			object = "pod"
			pod = args[0]
			command = args[1:]
		}

		if object != "pod" {
			return errors.New("Non pod object not yet supported")
		}

		if len(command) == 0 {
			if tty {
				command = []string{"sh", "-c", "exec $(command -v bash || command -v ash || command -v sh)"}
			} else {
				return errors.New("Please specify a command")
			}
		}

		s := &cliSession{
			opts: Options{
				Command:          command,
				Container:        container,
				Kconfig:          kconfig,
				Namespace:        namespace,
				Object:           object,
				Pod:              pod,
				Stdin:            stdinFlag,
				TTY:              tty,
				noSanityCheck:    noSanityCheck,
				noTLSVerify:      noTLSVerify,
				directExec:       directExec,
				directExecNodeIp: directExecNodeIp,
				base64SubProto:   base64SubProto,
			},
		}

		if s.opts.noSanityCheck && s.opts.directExec {
			if s.opts.directExecNodeIp == "" {
				return errors.New("When using direct-exec you must either allow preflight request or provide node IP via --node-direct-exec-ip")
			}
			if s.opts.Container == "" {
				return errors.New("When using direct-exec you must either allow preflight request or provide target container name via -c")
			}
		}

		// propogate logging flags
		flag.Set("v", fmt.Sprint(loglevel))
		flag.Set("stderrthreshold", fmt.Sprint(loglevel))

		err := s.prepConfig()
		if err != nil {
			return err
		}

		var req *http.Request
		if s.opts.directExec {
			req, err = s.prepKubeletExec()
			if err != nil {
				return err
			}

		} else {
			req, err = s.prepExec()
			if err != nil {
				return err
			}
		}
		return s.doExec(req)

	},
}

func Execute() {
	klog.InitFlags(nil)

	err := rootCmd.Execute()
	if err != nil {
		klog.Exit(err)
	}
	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kconfig, "kubeconfig", "", "kubeconfig file (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Set namespace")
	rootCmd.PersistentFlags().IntVarP(&loglevel, "loglevel", "v", 2, "Set loglevel")
	rootCmd.PersistentFlags().BoolVarP(&noTLSVerify, "skip-tls-verify", "k", false, "Don't perform TLS certificate verifiation")

	rootCmd.Flags().BoolVarP(&tty, "tty", "t", false, "Stdin is a TTY")
	rootCmd.Flags().BoolVarP(&stdinFlag, "stdin", "i", false, "Pass stdin to container")
	rootCmd.Flags().StringVarP(&container, "container", "c", "", "Container name")
	rootCmd.Flags().BoolVar(&noSanityCheck, "no-sanity-check", false, "Don't make preflight request to ensure pod exists")
	rootCmd.Flags().BoolVar(&directExec, "node-direct-exec", false, "Partially bypass the API server, by using the kubelet API")
	rootCmd.Flags().StringVar(&directExecNodeIp, "node-direct-exec-ip", "", "Node IP to use with direct-exec feature")
	rootCmd.Flags().BoolVar(&base64SubProto, "base64", false, "Use base64 subprotocol by default")
}
