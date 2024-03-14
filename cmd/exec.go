package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/moby/term"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type Options struct {
	Command          []string
	Container        string
	Kconfig          string
	Namespace        string
	Object           string
	Pod              string
	Stdin            bool
	TTY              bool
	PodSpec          corev1.PodSpec
	noSanityCheck    bool
	noTLSVerify      bool
	directExec       bool
	directExecNodeIp string
}

var protocols = []string{
	"v4.channel.k8s.io",
	"v3.channel.k8s.io",
	"v2.channel.k8s.io",
	"channel.k8s.io",
}

// https://github.com/kubernetes/kubernetes/blob/1a2f167d399b046bea6192df9e9b1ca7ac4f2365/staging/src/k8s.io/client-go/tools/remotecommand/remotecommand_websocket.go#L35
const (
	streamStdIn  = 0
	streamStdOut = 1
	streamStdErr = 2
	streamErr    = 3
	streamResize = 4
)

type cliSession struct {
	opts       Options
	clientConf *rest.Config
	k8sClient  *kubernetes.Clientset
	namespace  string
	RawMode    bool
}

func NewCliSession(o *Options) (*cliSession, error) {
	c := &cliSession{
		opts: *o,
	}

	err := c.prepConfig()
	if err != nil {
		return nil, err
	}

	c.k8sClient, err = kubernetes.NewForConfig(c.clientConf)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// prep the session
func (c *cliSession) prepConfig() error {
	var cfg clientcmd.ClientConfig
	switch c.opts.Kconfig {
	case "":
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{})
	default:
		cfg = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: c.opts.Kconfig},
			&clientcmd.ConfigOverrides{})
	}
	cc, err := cfg.ClientConfig()
	if err != nil {
		return err
	}
	c.clientConf = cc

	switch c.opts.Namespace {
	case "":
		c.namespace, _, err = cfg.Namespace()
		if err != nil {
			return err
		}
	default:
		c.namespace = c.opts.Namespace
	}

	if c.opts.noTLSVerify {
		c.clientConf.TLSClientConfig.Insecure = true
		c.clientConf.TLSClientConfig.CAFile = ""
		c.clientConf.TLSClientConfig.CAData = []byte("")
	}

	c.clientConf.UserAgent = fmt.Sprintf("kubectl-execws/%s", releaseVersion)

	return nil
}

func (c *cliSession) sanityCheck() error {
	if !c.opts.noSanityCheck {
		res, err := c.k8sClient.CoreV1().Pods(c.namespace).Get(context.TODO(), c.opts.Pod, metav1.GetOptions{})
		if err != nil {
			return err
		}
		c.opts.PodSpec = res.Spec
	}
	return nil
}

func (c *cliSession) prepExec() (*http.Request, error) {
	u, err := url.Parse(c.clientConf.Host)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, errors.New("Cannot determine websocket scheme")
	}

	u.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", c.namespace, c.opts.Pod)
	query := url.Values{}
	query.Add("stdout", "true")
	query.Add("stderr", "true")

	for _, c := range c.opts.Command {
		query.Add("command", c)
	}

	if c.opts.Container != "" {
		query.Add("container", c.opts.Container)
	}

	if c.opts.TTY {
		stdIn, _, _ := term.StdStreams()
		_, c.RawMode = term.GetFdInfo(stdIn)
		if !c.RawMode {
			klog.V(2).Infof("Unable to use a TTY - input is not a terminal or the right kind of file")
		}
		query.Add("tty", fmt.Sprintf("%t", c.RawMode))
	}

	if c.opts.Stdin {
		query.Add("stdin", "true")
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	return req, nil

}

// req -> ws callback
func (c *cliSession) doExec(req *http.Request) error {
	tlsConfig, err := rest.TLSConfigFor(c.clientConf)
	if err != nil {
		return err
	}

	dialer := &websocket.Dialer{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: tlsConfig,
		Subprotocols:    protocols,
	}

	initState := &TerminalState{
		IsRaw: c.RawMode,
	}
	if c.RawMode {
		stdIn, stdOut, _ := term.StdStreams()
		stdInFd, _ := term.GetFdInfo(stdIn)
		stdOutFd, _ := term.GetFdInfo(stdOut)
		initState.StdInFd = stdInFd
		initState.StdOutFd = stdOutFd
		initState.StateBlob, err = term.SetRawTerminal(stdInFd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(stdInFd, initState.StateBlob)
	}

	rt := &WebsocketRoundTripper{
		Dialer:    dialer,
		TermState: initState,
	}

	rter, err := rest.HTTPWrappersForConfig(c.clientConf, rt)
	if err != nil {
		return err
	}

	_, err = rter.RoundTrip(req)
	if err != nil {
		return err

	}
	return nil
}
