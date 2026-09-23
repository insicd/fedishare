package tunnel

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Conn is a framed bidirectional tunnel.
type Conn struct {
	nc net.Conn
	r  *bufio.Reader
	w  *bufio.Writer
	mu sync.Mutex
}

func NewConn(nc net.Conn) *Conn {
	return &Conn{
		nc: nc,
		r:  bufio.NewReaderSize(nc, 64<<10),
		w:  bufio.NewWriterSize(nc, 64<<10),
	}
}

func NewConnBuf(nc net.Conn, r *bufio.Reader) *Conn {
	if r == nil {
		r = bufio.NewReaderSize(nc, 64<<10)
	}
	return &Conn{
		nc: nc,
		r:  r,
		w:  bufio.NewWriterSize(nc, 64<<10),
	}
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.nc.SetDeadline(t)
}

func (c *Conn) Close() error {
	return c.nc.Close()
}

func (c *Conn) ReadFrame() (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[0:4])
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("frame too large: %d", n)
	}
	typ := hdr[4]
	payload := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(c.r, payload); err != nil {
			return 0, nil, err
		}
	}
	return typ, payload, nil
}

func (c *Conn) WriteFrame(typ byte, payload []byte) error {
	if len(payload) > MaxFrame {
		return fmt.Errorf("frame too large: %d", len(payload))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var hdr [5]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	hdr[4] = typ
	if _, err := c.w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.w.Write(payload); err != nil {
			return err
		}
	}
	return c.w.Flush()
}
