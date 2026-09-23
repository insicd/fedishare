package activitypub

import "testing"

func TestParseResourceAcctAndActorURL(t *testing.T) {
	acct, err := parseResource("acct:Alice@nodes.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if acct.Username != "alice" || acct.Host != "nodes.example.org" {
		t.Fatalf("%+v", acct)
	}

	withPort, err := parseResource("acct:alice@127.0.0.1:17890")
	if err != nil {
		t.Fatal(err)
	}
	if withPort.Host != "127.0.0.1" {
		t.Fatalf("host=%s", withPort.Host)
	}

	actor, err := parseResource("https://nodes.example.org/users/alice")
	if err != nil {
		t.Fatal(err)
	}
	if actor.Username != "alice" || actor.Host != "nodes.example.org" || actor.ActorURL == "" {
		t.Fatalf("%+v", actor)
	}
}

func TestParseResourceRejectsGarbage(t *testing.T) {
	for _, raw := range []string{
		"",
		"alice",
		"acct:alice",
		"acct:@nodes.example.org",
		"https://nodes.example.org/other/alice",
		"https://nodes.example.org/users/alice/outbox",
		"acct:alice@nodes.example.org extra",
	} {
		if _, err := parseResource(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestResourceMatchesCanonicalAndLoopback(t *testing.T) {
	parsed, err := parseResource("acct:alice@nodes.example.org")
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.matches("alice", "nodes.example.org", "https://nodes.example.org/users/alice", "127.0.0.1") {
		t.Fatal("canonical acct should match")
	}
	if parsed.matches("bob", "nodes.example.org", "https://nodes.example.org/users/bob", "127.0.0.1") {
		t.Fatal("wrong user")
	}

	local, err := parseResource("acct:alice@127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !local.matches("alice", "nodes.example.org", "https://nodes.example.org/users/alice", "127.0.0.1") {
		t.Fatal("loopback acct should match when the request is local")
	}

	evil, err := parseResource("acct:alice@evil.example")
	if err != nil {
		t.Fatal(err)
	}
	if evil.matches("alice", "nodes.example.org", "https://nodes.example.org/users/alice", "127.0.0.1") {
		t.Fatal("foreign host must not match")
	}
}
