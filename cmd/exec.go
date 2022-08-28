package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type Options struct {
	Command       []string
	Container     string
	Kconfig       string
	Namespace     string
	Object        string
	Pod           string
	Stdin         bool
	TTY           bool
	noSanityCheck bool
	noTLSVerify   bool
}

var protocols = []string{
	"v4.channel.k8s.io",
	"v3.channel.k8s.io",
	"v2.channel.k8s.io",
	"channel.k8s.io",
}

const (
	stdin = iota
	stdout
	stderr
)

type cliSession struct {
	opts       Options
	clientConf *rest.Config
	namespace  string
}

type RoundTripCallback func(conn *websocket.Conn) error

type WebsocketRoundTripper struct {
	Dialer   *websocket.Dialer
	Callback RoundTripCallback
}

type ApiServerError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
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

	if !c.opts.noSanityCheck {
		client, err := kubernetes.NewForConfig(c.clientConf)
		if err != nil {
			return err
		}

		_, err = client.CoreV1().Pods(c.namespace).Get(context.TODO(), c.opts.Pod, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// prep a http req
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
		return nil, fmt.Errorf("Malformed URL %s", u.String())
	}

	u.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", c.namespace, c.opts.Pod)
	rawQuery := "stdout=true&stderr=true"
	for _, c := range c.opts.Command {
		rawQuery += "&command=" + c
	}

	if c.opts.Container != "" {
		rawQuery += "&container=" + c.opts.Container
	}

	if c.opts.TTY {
		rawQuery += "&tty=true"
		klog.Warning("Raw terminal not supported yet, YMMV.")
	}

	if c.opts.Stdin {
		rawQuery += "&stdin=true"
	}
	u.RawQuery = rawQuery

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
	}

	return req, nil

}

//req -> ws callback
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

	rt := &WebsocketRoundTripper{
		Callback: WsCallback,
		Dialer:   dialer,
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

func (d *WebsocketRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	conn, resp, err := d.Dialer.Dial(r.URL.String(), r.Header)
	if e, ok := err.(*net.OpError); ok {
		return nil, fmt.Errorf("Error connecting to %s, %s", e.Addr, e.Err)
	} else if err != nil {
		return nil, err
	} else if resp.StatusCode != 101 {
		var msg ApiServerError
		err := json.NewDecoder(resp.Body).Decode(&msg)
		if err != nil {
			return nil, errors.New("Error from server, unable to decode response")
		}
		return nil, fmt.Errorf("Error from server (%s): %s", msg.Reason, msg.Message)
	}
	defer conn.Close()
	return resp, d.Callback(conn)
}

func WsCallback(ws *websocket.Conn) error {
	errChan := make(chan error, 3)
	var sendBuffer bytes.Buffer

	wg := sync.WaitGroup{}
	wg.Add(2)

	// send
	go func() {
		defer wg.Done()
		buf := make([]byte, 1025)
		for {
			n, err := os.Stdin.Read(buf[1:])
			if err != nil {
				errChan <- err
				return
			}

			sendBuffer.Write(buf[1:n])
			sendBuffer.Write([]byte{13, 10})
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n+1])
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	// recv
	go func() {
		defer wg.Done()
		for {
			msgType, buf, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if msgType != websocket.BinaryMessage {
				errChan <- errors.New("Received unexpected websocket message")
				return
			}

			if len(buf) > 1 {
				var w io.Writer
				switch buf[0] {
				case stdout:
					w = os.Stdout
				case stderr:
					w = os.Stderr
				}

				if w == nil {
					continue
				}

				// ash terminal hack
				b := bytes.Replace(buf[1:], []byte("\x1b\x5b\x36\x6e"), []byte(""), -1)
				out := bytes.Replace(b, sendBuffer.Bytes(), []byte(""), -1)

				_, err = w.Write(out)
				if err != nil {
					errChan <- err
					return
				}
			}
			sendBuffer.Reset()
		}
	}()

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if e, ok := err.(*websocket.CloseError); ok {
			klog.V(4).Infof("Closing websocket connection with error code %d, err: %s", e.Code, err)
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return nil
		} else if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}
