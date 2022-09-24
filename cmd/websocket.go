package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/moby/term"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

type WebsocketRoundTripper struct {
	Dialer    *websocket.Dialer
	TermState *TerminalState
	opts      Options
}

type ApiServerError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func (d *WebsocketRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	conn, resp, err := d.Dialer.Dial(r.URL.String(), r.Header)
	if e, ok := err.(*net.OpError); ok {
		return nil, fmt.Errorf("Error connecting to %s, %s", e.Addr, e.Err)
	} else if err != nil || resp.StatusCode != 101 {
		if resp.Header.Get("Content-Type") == "application/json" {
			var msg ApiServerError
			jerr := json.NewDecoder(resp.Body).Decode(&msg)
			if jerr != nil {
				return nil, errors.Wrap(err, "Error from server, unable to decode response")
			}
			return nil, fmt.Errorf("Error from server (%s): %s", msg.Reason, msg.Message)
		} else {
			body, ioerr := ioutil.ReadAll(resp.Body)
			if ioerr != nil {
				return nil, errors.Wrap(err, "Server Error, unable to read body")
			}
			resp.Body.Close()

			return nil, fmt.Errorf("Error from server: %s", body)
		}
	}
	defer conn.Close()
	return resp, d.WsCallback(conn)
}

func (d *WebsocketRoundTripper) WsCallback(ws *websocket.Conn) error {
	errChan := make(chan error, 4)
	var sendBuffer bytes.Buffer

	wg := sync.WaitGroup{}
	wg.Add(3)

	stdIn, stdOut, stdErr := term.StdStreams()

	// send
	go func() {
		defer wg.Done()
		buf := make([]byte, 1025)
		for {
			n, err := stdIn.Read(buf[1:])
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

			var stream uint64
			var contents []byte

			switch msgType {
			case websocket.BinaryMessage:
				stream = uint64(buf[0])
				contents = buf[1:]
			case websocket.TextMessage:
				raw := make([]byte, len(buf))
				_, err = base64.StdEncoding.Decode(buf[1:], raw)
				if err != nil {
					errChan <- err
					return
				}
				stream, err = strconv.ParseUint(string(buf[0]), 10, 16)
				contents = raw[1:]

			default:
				errChan <- fmt.Errorf("Received unexpected websocket message: type %d", msgType)
				return
			}

			if len(contents) > 0 {
				var w io.Writer
				switch stream {
				case streamStdOut:
					w = stdOut
				case streamStdErr:
					w = stdErr
				case streamErr:
					if err := parseStreamErr(buf[1:]); err != nil {
						errChan <- err
						return
					}
				default:
					errChan <- fmt.Errorf("Unknown stream type: %d", stream)
					continue
				}

				if w == nil {
					continue
				}

				_, err = w.Write(contents)
				if err != nil {
					errChan <- err
					return
				}
			}
			sendBuffer.Reset()
		}
	}()

	// resize
	go func() {
		defer wg.Done()
		if d.opts.TTY {
			resizeNotify := registerResizeSignal()

			d.TermState.Initialised = false
			for {
				changed, err := updateSize(d.TermState)
				if err != nil {
					errChan <- fmt.Errorf("Failed to update terminal size: %w", err)
					return
				}

				if changed || !d.TermState.Initialised {
					res, err := json.Marshal(d.TermState.Size)
					if err != nil {
						errChan <- fmt.Errorf("Failed to marshal JSON: %w", err)
						return
					}
					msg := []byte(fmt.Sprintf("%s%s", "\x04", res))

					err = ws.WriteMessage(websocket.BinaryMessage, msg)
					if err != nil {
						errChan <- fmt.Errorf("Failed to write msg to channel: %w", err)
						return
					}
					d.TermState.Initialised = true
				}

				waitForResizeChange(resizeNotify)
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

type streamError struct {
	Status  string             `json:"status"`
	Message string             `json:"message"`
	Reason  string             `json:"reason"`
	Details streamErrorDetails `json:"details"`
}

type streamErrorDetails struct {
	Causes []streamErrorReason `json:"causes"`
}

type streamErrorReason struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func parseStreamErr(buf []byte) error {
	var msg streamError
	jerr := json.Unmarshal(buf, &msg)
	if jerr != nil {
		return errors.Wrap(jerr, "Error from server, unable to decode response")
	}

	if msg.Status == "Success" {
		return nil
	}

	if msg.Status == "Failure" && msg.Reason == "NonZeroExitCode" {
		exit, _ := strconv.Atoi(msg.Details.Causes[0].Message)
		return fmt.Errorf("command terminated with exit code %d", exit)
	}

	return fmt.Errorf("error: %s", msg.Message)
}
