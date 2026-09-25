package network

import "testing"

func TestActorFromDocument(t *testing.T) {
	a := ActorFromDocument("alice", "https://nodes.example.org", `{
		"name":"Alice",
		"preferredUsername":"alice",
		"summary":"<p>Sharing files.</p>",
		"id":"https://nodes.example.org/users/alice"
	}`, true)
	if a.Acct != "alice@nodes.example.org" || a.Name != "Alice" || a.Summary != "Sharing files." {
		t.Fatalf("%+v", a)
	}
	if !a.Online || a.URL != "https://nodes.example.org/users/alice" {
		t.Fatalf("%+v", a)
	}
	empty := ActorFromDocument("bob", "https://nodes.example.org", "", false)
	if empty.Name != "bob" || empty.Online || empty.URL != "https://nodes.example.org/users/bob" {
		t.Fatalf("%+v", empty)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in   string
		kind Kind
		out  string
	}{
		{"", KindEmpty, ""},
		{"https://nodes.example.org", KindGateway, "https://nodes.example.org"},
		{"https://nodes.example.org/", KindGateway, "https://nodes.example.org"},
		{"https://nodes.example.org/.well-known/fedishare-network", KindGateway, "https://nodes.example.org"},
		{"https://nodes.example.org/users/alice", KindActor, "https://nodes.example.org/users/alice"},
		{"@alice@nodes.example.org", KindAcct, "alice@nodes.example.org"},
		{"acct:alice@nodes.example.org", KindAcct, "alice@nodes.example.org"},
		{"alice", KindAcct, "alice"},
		{"not a handle", KindUnknown, "not a handle"},
	}
	for _, tc := range cases {
		kind, value := Classify(tc.in)
		if kind != tc.kind || value != tc.out {
			t.Fatalf("%q => %d %q, want %d %q", tc.in, kind, value, tc.kind, tc.out)
		}
	}
}
