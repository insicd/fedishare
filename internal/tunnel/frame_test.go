package tunnel

import (
	"net"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	errc := make(chan error, 1)
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		defer nc.Close()
		cb := NewConn(nc)
		typ, payload, err := cb.ReadFrame()
		if err != nil {
			errc <- err
			return
		}
		if typ != TypePing || string(payload) != "hi" {
			errc <- errString("mismatch")
			return
		}
		errc <- cb.WriteFrame(TypePong, []byte("ok"))
	}()

	nc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	ca := NewConn(nc)
	if err := ca.WriteFrame(TypePing, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	typ, payload, err := ca.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if typ != TypePong || string(payload) != "ok" {
		t.Fatalf("got %d %q", typ, payload)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
