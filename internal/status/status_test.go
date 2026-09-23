package status

import (
	"testing"
	"time"
)

func TestValidTransitions(t *testing.T) {
	s := New()
	if s.Snapshot().State != StateStarting {
		t.Fatal("initial state")
	}
	seq := []State{StateOffline, StateConnecting, StateOnline, StatePaused, StateOnline, StateShuttingDown}
	for _, next := range seq {
		if err := s.SetState(next); err != nil {
			t.Fatalf("to %s: %v", next, err)
		}
	}
}

func TestInvalidTransition(t *testing.T) {
	s := New()
	if err := s.SetState(StateShuttingDown); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState(StateOnline); err == nil {
		t.Fatal("shutting down is terminal")
	}
}

func TestSetError(t *testing.T) {
	s := New()
	if err := s.SetError("Could not open the database."); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.State != StateError || snap.Error == "" {
		t.Fatalf("%+v", snap)
	}
}

func TestWatchReceivesUpdates(t *testing.T) {
	s := New()
	ch, unsub := s.Watch()
	defer unsub()

	select {
	case snap := <-ch:
		if snap.State != StateStarting {
			t.Fatalf("first snap %s", snap.State)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for initial snapshot")
	}

	if err := s.SetState(StateOffline); err != nil {
		t.Fatal(err)
	}
	select {
	case snap := <-ch:
		if snap.State != StateOffline {
			t.Fatalf("got %s", snap.State)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for update")
	}
}

func TestSnapshotIsolation(t *testing.T) {
	s := New()
	snap := s.Snapshot()
	snap.IndexedFiles = 99
	if s.Snapshot().IndexedFiles != 0 {
		t.Fatal("snapshot mutation leaked")
	}
}

func TestLabels(t *testing.T) {
	if StateOnline.Label() != "Online" {
		t.Fatal(StateOnline.Label())
	}
	if StateOffline.String() != "offline" {
		t.Fatal(StateOffline.String())
	}
}
