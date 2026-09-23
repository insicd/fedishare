package tunnel

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client is the desktop side of a reverse tunnel.
type Client struct {
	GatewayURL   string
	Username     string
	PublicKeyPEM string
	NodeID       string
	DisplayName  string
	Key          *rsa.PrivateKey
	Log          *slog.Logger

	conn *Conn
}

// Connect upgrades to the gateway and completes challenge-response auth.
func (c *Client) Connect(ctx context.Context) error {
	if c.Log == nil {
		c.Log = slog.Default()
	}
	conn, err := DialUpgrade(ctx, c.GatewayURL)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	hello, err := json.Marshal(Hello{
		Username:    strings.ToLower(strings.TrimSpace(c.Username)),
		PublicKey:   c.PublicKeyPEM,
		NodeID:      c.NodeID,
		DisplayName: c.DisplayName,
	})
	if err != nil {
		_ = conn.Close()
		return err
	}
	if err := conn.WriteFrame(TypeHello, hello); err != nil {
		_ = conn.Close()
		return err
	}
	typ, payload, err := conn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return err
	}
	if typ != TypeChallenge {
		_ = conn.Close()
		return fmt.Errorf("expected challenge, got type %d", typ)
	}
	var ch Challenge
	if err := json.Unmarshal(payload, &ch); err != nil {
		_ = conn.Close()
		return err
	}
	sig, err := SignChallenge(c.Key, c.Username, ch.Nonce, ch.IssuedAt)
	if err != nil {
		_ = conn.Close()
		return err
	}
	auth, err := json.Marshal(Auth{Signature: sig})
	if err != nil {
		_ = conn.Close()
		return err
	}
	if err := conn.WriteFrame(TypeAuth, auth); err != nil {
		_ = conn.Close()
		return err
	}
	typ, payload, err = conn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	switch typ {
	case TypeAuthOK:
		c.conn = conn
		return nil
	case TypeAuthFail:
		var fail AuthFail
		_ = json.Unmarshal(payload, &fail)
		_ = conn.Close()
		if fail.Error == "" {
			fail.Error = "gateway rejected the tunnel"
		}
		return fmt.Errorf("%s", fail.Error)
	default:
		_ = conn.Close()
		return fmt.Errorf("unexpected auth reply type %d", typ)
	}
}

// Serve handles proxied HTTP until the connection or context ends.
func (c *Client) Serve(ctx context.Context, handler http.Handler) error {
	if c.conn == nil {
		return fmt.Errorf("tunnel is not connected")
	}
	if handler == nil {
		return fmt.Errorf("missing public handler")
	}
	go func() {
		<-ctx.Done()
		_ = c.conn.Close()
	}()
	done := make(chan struct{})
	defer close(done)
	go c.pingLoop(ctx, done)

	for {
		typ, payload, err := c.conn.ReadFrame()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		switch typ {
		case TypePing:
			_ = c.conn.WriteFrame(TypePong, nil)
		case TypePong:
		case TypeHTTPReq:
			var req HTTPReq
			if err := json.Unmarshal(payload, &req); err != nil {
				c.Log.Debug("bad proxied request", "err", err)
				continue
			}
			go c.handleHTTP(ctx, handler, req)
		default:
			c.Log.Debug("ignored tunnel frame", "type", typ)
		}
	}
}

func (c *Client) pingLoop(ctx context.Context, done <-chan struct{}) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-t.C:
			_ = c.conn.WriteFrame(TypePing, nil)
		}
	}
}

func (c *Client) handleHTTP(ctx context.Context, handler http.Handler, in HTTPReq) {
	if in.ID == "" || len(in.ID) != 16 {
		return
	}
	path := in.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := path
	if in.Query != "" {
		u += "?" + in.Query
	}
	var body io.Reader = http.NoBody
	if len(in.Body) > 0 {
		body = bytes.NewReader(in.Body)
	}
	req, err := http.NewRequestWithContext(ctx, in.Method, "http://gateway.invalid"+u, body)
	if err != nil {
		c.writeError(in.ID, http.StatusBadRequest, "invalid request")
		return
	}
	req.Header = StripHop(http.Header(in.Header))
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
	}
	req.RemoteAddr = "gateway"
	req.RequestURI = u
	w := &streamWriter{conn: c.conn, id: in.ID, header: make(http.Header)}
	handler.ServeHTTP(w, req)
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	end, _ := json.Marshal(HTTPResEnd{ID: in.ID})
	_ = c.conn.WriteFrame(TypeHTTPResEnd, end)
}

func (c *Client) writeError(id string, status int, msg string) {
	head, _ := json.Marshal(HTTPResHead{
		ID:     id,
		Status: status,
		Header: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
	})
	_ = c.conn.WriteFrame(TypeHTTPResHead, head)
	_ = c.conn.WriteFrame(TypeHTTPResBody, encodeBody(id, []byte(msg)))
	end, _ := json.Marshal(HTTPResEnd{ID: id})
	_ = c.conn.WriteFrame(TypeHTTPResEnd, end)
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

type streamWriter struct {
	conn   *Conn
	id     string
	header http.Header
	wrote  bool
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	head, err := json.Marshal(HTTPResHead{ID: w.id, Status: code, Header: StripHop(w.header)})
	if err != nil {
		return
	}
	_ = w.conn.WriteFrame(TypeHTTPResHead, head)
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	wrote := 0
	for len(p) > 0 {
		n := Chunk
		if n > len(p) {
			n = len(p)
		}
		if err := w.conn.WriteFrame(TypeHTTPResBody, encodeBody(w.id, p[:n])); err != nil {
			return wrote, err
		}
		wrote += n
		p = p[n:]
	}
	return wrote, nil
}

func (w *streamWriter) Flush() {}

func encodeBody(id string, data []byte) []byte {
	out := make([]byte, 16+len(data))
	copy(out[:16], id)
	copy(out[16:], data)
	return out
}
