package security

import (
	"net"
	"testing"
)

func TestPolicyBlocksPrivateAndMetadata(t *testing.T) {
	p := Policy{}
	for _, raw := range []string{
		"http://127.0.0.1/inbox",
		"http://localhost/x",
		"http://10.0.0.5/x",
		"http://192.168.1.1/x",
		"http://169.254.169.254/latest/meta-data",
		"http://metadata.google.internal/",
		"ftp://example.com/x",
	} {
		if err := p.CheckURL(raw); err == nil {
			t.Fatalf("expected block for %s", raw)
		}
	}
	if err := p.CheckURL("https://example.org/users/bob"); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyAllowsPrivateWhenRequested(t *testing.T) {
	p := Policy{AllowPrivate: true}
	if err := p.CheckURL("http://127.0.0.1:8080/inbox"); err != nil {
		t.Fatal(err)
	}
	if p.blockedIP(net.ParseIP("10.1.2.3")) {
		t.Fatal("allow private")
	}
}

func TestHostOnly(t *testing.T) {
	if HostOnly("https://nodes.example.org/users/alice") != "nodes.example.org" {
		t.Fatal(HostOnly("https://nodes.example.org/users/alice"))
	}
}
