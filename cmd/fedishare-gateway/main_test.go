package main

import "testing"

func TestEnvOr(t *testing.T) {
	t.Setenv("FEDISHARE_LISTEN", "0.0.0.0:9")
	if got := envOr("FEDISHARE_LISTEN", "127.0.0.1:8080"); got != "0.0.0.0:9" {
		t.Fatalf("got %q", got)
	}
	if got := envOr("FEDISHARE_GATEWAY_NO_SUCH", "fallback"); got != "fallback" {
		t.Fatalf("got %q", got)
	}
}

func TestFirstEnv(t *testing.T) {
	t.Setenv("FEDISHARE_PUBLIC_URL", "")
	t.Setenv("CLOUDRON_APP_ORIGIN", "https://nodes.example.org")
	if got := firstEnv("FEDISHARE_PUBLIC_URL", "CLOUDRON_APP_ORIGIN"); got != "https://nodes.example.org" {
		t.Fatalf("got %q", got)
	}
}
