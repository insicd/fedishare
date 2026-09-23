package files

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type Limits struct {
	MaxConcurrent int
	PerClient     int
	BandwidthBPS  int64
}

type Limiter struct {
	Limits Limits

	mu      sync.Mutex
	active  int
	perHost map[string]int
}

func NewLimiter(l Limits) *Limiter {
	if l.MaxConcurrent <= 0 {
		l.MaxConcurrent = 8
	}
	if l.PerClient <= 0 {
		l.PerClient = 4
	}
	return &Limiter{Limits: l, perHost: make(map[string]int)}
}

func (l *Limiter) Acquire(r *http.Request) (func(), error) {
	host := clientHost(r)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active >= l.Limits.MaxConcurrent {
		return nil, fmt.Errorf("too many downloads")
	}
	if l.perHost[host] >= l.Limits.PerClient {
		return nil, fmt.Errorf("too many downloads from this client")
	}
	l.active++
	l.perHost[host]++
	return func() {
		l.mu.Lock()
		l.active--
		l.perHost[host]--
		if l.perHost[host] <= 0 {
			delete(l.perHost, host)
		}
		l.mu.Unlock()
	}, nil
}

func (l *Limiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

func clientHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func LimitReader(r io.Reader, bps int64) io.Reader {
	if bps <= 0 {
		return r
	}
	return &pacedReader{r: r, bps: bps, start: time.Now()}
}

type pacedReader struct {
	r     io.Reader
	bps   int64
	got   int64
	start time.Time
}

func (p *pacedReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.got += int64(n)
	want := time.Duration(p.got) * time.Second / time.Duration(p.bps)
	if d := want - time.Since(p.start); d > 0 {
		time.Sleep(d)
	}
	return n, err
}
