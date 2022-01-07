package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Options struct {
	Command   []string
	Container string
	Kconfig   string
	Namespace string
	Object    string
	Pod       string
	Stdin     bool
	TTY       bool
}

var cfg clientcmd.ClientConfig

var protocols = []string{
	"v4.channel.k8s.io",
	"v3.channel.k8s.io",
	"v2.channel.k8s.io",
	"channel.k8s.io",
}

var cacheBuff bytes.Buffer

const (
	stdin = iota
	stdout
	stderr
)

// prep a http req
func prepExec(opts *Options) (*http.Request, error) {
	//var cfg clientcmd.ClientConfig
	switch opts.Kconfig {
	case "":
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{})
	default:
		cfg = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kconfig},
			&clientcmd.ConfigOverrides{})
	}
	clientConf, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}

	var namespace string
	switch opts.Namespace {
	case "":
		namespace, _, err = cfg.Namespace()
		if err != nil {
			return nil, err
		}
	default:
		namespace = opts.Namespace
	}

	u, err := url.Parse(clientConf.Host)
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

	u.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", namespace, opts.Pod)
	rawQuery := "stdout=true&stderr=true"
	for _, c := range opts.Command {
		rawQuery += "&command=" + c
	}

	if opts.Container != "" {
		rawQuery += "&container=" + opts.Container
	}

	if opts.TTY {
		rawQuery += "&tty=true"
	}

	if opts.Stdin {
		rawQuery += "&stdin=true"
	}
	u.RawQuery = rawQuery

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
	}

	return req, nil

}

type RoundTripCallback func(conn *websocket.Conn) error

type WebsocketRoundTripper struct {
	Dialer   *websocket.Dialer
	Callback RoundTripCallback
}

//req -> ws callback
func doExec(req *http.Request) error {
	config, err := cfg.ClientConfig()
	if err != nil {
		return err
	}

	tlsConfig, err := rest.TLSConfigFor(config)
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

	rter, err := rest.HTTPWrappersForConfig(config, rt)
	if err != nil {
		return err
	}

	_, err = rter.RoundTrip(req)
	if err != nil {
		return err

	}
	return nil
}

type ApiServerError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func (d *WebsocketRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	conn, resp, err := d.Dialer.Dial(r.URL.String(), r.Header)
	if err != nil {
		var msg ApiServerError
		err := json.NewDecoder(resp.Body).Decode(&msg)
		// should probably match 400-599 here
		if resp.StatusCode != 101 || err != nil {
			errmsg := fmt.Sprintf("Error from server (%s): %s", msg.Reason, msg.Message)
			return nil, errors.New(errmsg)
		} else {
			return nil, err
		}
	}
	defer conn.Close()
	return resp, d.Callback(conn)
}

func WsCallback(ws *websocket.Conn) error {
	errChan := make(chan error, 3)
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1025)
		for {
			n, err := os.Stdin.Read(buf[1:])
			if err != nil {
				errChan <- err
				return
			}

			cacheBuff.Write(buf[1:n])
			cacheBuff.Write([]byte{13, 10})
			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n+1]); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			_, buf, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
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
				s := strings.Replace(string(buf[1:]), cacheBuff.String(), "", -1)
				_, err = w.Write([]byte(s))
				if err != nil {
					errChan <- err
					return
				}
			}
			cacheBuff.Reset()
		}
	}()

	wg.Wait()
	close(errChan)
        err := <-errChan
	if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		return nil
	} else {
		return err
	}
}
