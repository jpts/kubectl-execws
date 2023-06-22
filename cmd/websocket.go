package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/moby/term"
	"k8s.io/klog/v2"
)

type WebsocketRoundTripper struct {
	Dialer     *websocket.Dialer
	TermState  *TerminalState
	opts       Options
	SendBuffer bytes.Buffer
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
				return nil, fmt.Errorf("Error from server, unable to decode response: %w", err)
			}
			return nil, fmt.Errorf("Error from server (%s): %s", msg.Reason, msg.Message)
		} else {
			body, ioerr := ioutil.ReadAll(resp.Body)
			if ioerr != nil {
				return nil, fmt.Errorf("Server Error, unable to read body: %w", err)
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

	wg := sync.WaitGroup{}
	wg.Add(3)

	go d.concurrentSend(&wg, ws, errChan)
	go d.concurrentRecv(&wg, ws, errChan)
	go d.concurrentResize(&wg, ws, errChan)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return nil
		} else if errors.Is(err, io.EOF) {
			klog.V(4).Info("Closing websocket connection with EOF")
			return nil
		}
		if e, ok := err.(*websocket.CloseError); ok {
			klog.V(4).Infof("Closing websocket connection with error code %d, err: %s", e.Code, err)
		}
		return err
	}
	return nil
}

func (d *WebsocketRoundTripper) concurrentSend(wg *sync.WaitGroup, ws *websocket.Conn, errChan chan error) {
	defer wg.Done()

	buf := make([]byte, 1025)
	stdIn, _, _ := term.StdStreams()

	for {
		n, err := stdIn.Read(buf[1:])
		if err != nil {
			errChan <- err
			return
		}

		d.SendBuffer.Write(buf[1:n])
		d.SendBuffer.Write([]byte{13, 10})
		err = ws.WriteMessage(websocket.BinaryMessage, buf[:n+1])
		if err != nil {
			errChan <- err
			return
		}
	}
}

func (d *WebsocketRoundTripper) concurrentRecv(wg *sync.WaitGroup, ws *websocket.Conn, errChan chan error) {
	defer wg.Done()

	_, stdOut, stdErr := term.StdStreams()

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
				w = stdOut
			case streamStdErr:
				w = stdErr
			case streamErr:
				if err := parseStreamErr(buf[1:]); err != nil {
					errChan <- err
					return
				}
			default:
				errChan <- fmt.Errorf("Unknown stream type: %d", buf[0])
				continue
			}

			if w == nil {
				continue
			}

			out := buf[1:]
			_, err = w.Write(out)
			if err != nil {
				errChan <- err
				return
			}
		}
		d.SendBuffer.Reset()
	}
}

func (d *WebsocketRoundTripper) concurrentResize(wg *sync.WaitGroup, ws *websocket.Conn, errChan chan error) {
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
		return fmt.Errorf("Error from server, unable to decode response: %w", jerr)
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
