// Package security holds network safety helpers used by federation.
package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrBlockedURL = errors.New("URL is not allowed")

// Policy decides which destinations federation HTTP may reach.
type Policy struct {
	// AllowPrivate permits loopback and RFC1918 addresses. Tests and
	// two local --data-dir nodes on one machine set this.
	AllowPrivate bool
}

func (p Policy) CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ErrBlockedURL
	}
	return p.CheckParsed(u)
}

func (p Policy) CheckParsed(u *url.URL) error {
	if u == nil || u.User != nil {
		return ErrBlockedURL
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ErrBlockedURL
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ErrBlockedURL
	}
	if host == "metadata.google.internal" || host == "metadata" ||
		strings.HasSuffix(host, ".internal") {
		return ErrBlockedURL
	}
	if !p.AllowPrivate && host == "localhost" {
		return ErrBlockedURL
	}
	if ip := net.ParseIP(host); ip != nil {
		if p.blockedIP(ip) {
			return ErrBlockedURL
		}
		return nil
	}
	return nil
}

func (p Policy) blockedIP(ip net.IP) bool {
	if p.AllowPrivate {
		return false
	}
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	return false
}

func (p Policy) lookupBlocked(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if p.blockedIP(ip) {
			return ErrBlockedURL
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return ErrBlockedURL
	}
	for _, ip := range ips {
		if p.blockedIP(ip.IP) {
			return ErrBlockedURL
		}
	}
	return nil
}

// DialContext resolves addr and refuses blocked IPs before connecting.
func (p Policy) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := p.lookupBlocked(ctx, host); err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// Client returns an HTTP client that cannot be redirected onto a blocked IP.
func (p Policy) Client(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           p.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if err := p.CheckParsed(req.URL); err != nil {
				return err
			}
			if err := p.lookupBlocked(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
}

// HostOnly returns the hostname for logs (never a full URL with credentials).
func HostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Hostname()
}
