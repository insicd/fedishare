package tunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DialUpgrade opens an outbound HTTP/1.1 Upgrade tunnel to gatewayURL.
func DialUpgrade(ctx context.Context, gatewayURL string) (*Conn, error) {
	u, err := url.Parse(strings.TrimSpace(gatewayURL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid gateway URL")
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		tlsConn := tls.Client(nc, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = nc.Close()
			return nil, err
		}
		nc = tlsConn
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = nc.SetDeadline(dl)
	} else {
		_ = nc.SetDeadline(time.Now().Add(20 * time.Second))
	}

	req := fmt.Sprintf("GET /v1/tunnel HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: %s\r\n\r\n",
		u.Host, UpgradeProtocol)
	if _, err := io.WriteString(nc, req); err != nil {
		_ = nc.Close()
		return nil, err
	}
	br := bufio.NewReader(nc)
	res, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		_ = nc.Close()
		return nil, fmt.Errorf("gateway upgrade: HTTP %d", res.StatusCode)
	}
	if !strings.EqualFold(res.Header.Get("Upgrade"), UpgradeProtocol) {
		_ = nc.Close()
		return nil, fmt.Errorf("gateway refused tunnel upgrade")
	}
	_ = nc.SetDeadline(time.Time{})
	return NewConnBuf(nc, br), nil
}

// AcceptUpgrade completes the server side of an HTTP Upgrade.
func AcceptUpgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), UpgradeProtocol) {
		return nil, fmt.Errorf("missing Upgrade: %s", UpgradeProtocol)
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response does not support hijack")
	}
	nc, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	_, err = io.WriteString(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: "+UpgradeProtocol+"\r\n\r\n")
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = nc.Close()
		return nil, err
	}
	return NewConnBuf(nc, rw.Reader), nil
}
