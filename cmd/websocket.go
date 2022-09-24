package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/moby/term"
	"k8s.io/klog/v2"
)

type WebsocketRoundTripper struct {
	Dialer    *websocket.Dialer
	Callback  RoundTripCallback
	TermState *TerminalState
	opts      Options
}

type RoundTripCallback func(ctx *WebsocketRoundTripper, conn *websocket.Conn) error

type ApiServerError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type TerminalState struct {
	Size        TerminalSize
	Fd          uintptr
	StateBlob   *term.State
	Initialised bool
}

type TerminalSize struct {
	Width  int `json:"Width"`
	Height int `json:"Height"`
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
	return resp, d.Callback(d, conn)
}

func WsCallbackWrapper(d *WebsocketRoundTripper, ws *websocket.Conn) error {
	return d.WsCallback(ws)
}

func (d *WebsocketRoundTripper) WsCallback(ws *websocket.Conn) error {
	errChan := make(chan error, 4)
	var sendBuffer bytes.Buffer

	wg := sync.WaitGroup{}
	wg.Add(3)

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
				case streamStdOut:
					w = os.Stdout
				case streamStdErr:
					w = os.Stderr
				case streamErr:
					w = os.Stderr
				default:
					errChan <- errors.New("Unknown stream type")
					continue
				}

				if w == nil {
					continue
				}

				_, err = w.Write(buf)
				if err != nil {
					errChan <- err
					return
				}
			}
			sendBuffer.Reset()
		}
	}()

	// TODO: remote error
	/*go func() {
	    }
	}()*/

	// resize
	go func() {
		defer wg.Done()
		if d.opts.TTY {
			resizeNotify := make(chan os.Signal, 1)
			signal.Notify(resizeNotify, syscall.SIGWINCH)

			d.TermState.Initialised = false
			for {
				changed, err := updateSize(d.TermState)
				if err != nil {
					errChan <- err
					return
				}

				if changed || !d.TermState.Initialised {
					res, err := json.Marshal(d.TermState.Size)
					if err != nil {
						errChan <- err
						return
					}
					msg := []byte(fmt.Sprintf("%s%s", "\x04", res))

					err = ws.WriteMessage(websocket.BinaryMessage, msg)
					if err != nil {
						errChan <- err
						return
					}
					d.TermState.Initialised = true
				}

				_ = <-resizeNotify
			}
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
