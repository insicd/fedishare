package status

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// State is the node lifecycle state consumed by the tray and dashboard.
type State int

const (
	StateStarting State = iota
	StateIndexing
	StateConnecting
	StateOnline
	StatePaused
	StateOffline
	StateError
	StateShuttingDown
)

func (s State) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateIndexing:
		return "indexing"
	case StateConnecting:
		return "connecting"
	case StateOnline:
		return "online"
	case StatePaused:
		return "paused"
	case StateOffline:
		return "offline"
	case StateError:
		return "error"
	case StateShuttingDown:
		return "shutting_down"
	default:
		return "unknown"
	}
}

func (s State) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Label is the short user-visible name.
func (s State) Label() string {
	switch s {
	case StateStarting:
		return "Starting"
	case StateIndexing:
		return "Indexing"
	case StateConnecting:
		return "Connecting"
	case StateOnline:
		return "Online"
	case StatePaused:
		return "Paused"
	case StateOffline:
		return "Offline"
	case StateError:
		return "Error"
	case StateShuttingDown:
		return "Shutting down"
	default:
		return "Unknown"
	}
}

var allowed = map[State][]State{
	StateStarting:     {StateIndexing, StateConnecting, StateOnline, StateOffline, StatePaused, StateError, StateShuttingDown},
	StateIndexing:     {StateIndexing, StateConnecting, StateOnline, StateOffline, StatePaused, StateError, StateShuttingDown},
	StateConnecting:   {StateConnecting, StateOnline, StateOffline, StatePaused, StateError, StateShuttingDown},
	StateOnline:       {StateIndexing, StateConnecting, StatePaused, StateOffline, StateError, StateShuttingDown},
	StatePaused:       {StateOnline, StateOffline, StateIndexing, StateConnecting, StateError, StateShuttingDown},
	StateOffline:      {StateConnecting, StateOnline, StateIndexing, StatePaused, StateError, StateShuttingDown},
	StateError:        {StateStarting, StateOffline, StateShuttingDown},
	StateShuttingDown: {},
}

func canTransition(from, to State) bool {
	if from == to {
		return true
	}
	for _, next := range allowed[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Snapshot is a point-in-time view of node health. Callers must treat it
// as immutable.
type Snapshot struct {
	State                 State     `json:"state"`
	Identity              string    `json:"identity"`
	NodeID                string    `json:"node_id"`
	ShareDirectory        string    `json:"share_directory"`
	IndexedFiles          int       `json:"indexed_files"`
	TotalBytes            int64     `json:"total_bytes"`
	GatewayConnected      bool      `json:"gateway_connected"`
	ActivityPubActive     bool      `json:"activitypub_active"`
	PendingFederationJobs int       `json:"pending_federation_jobs"`
	ActiveDownloads       int       `json:"active_downloads"`
	IndexingDone          int       `json:"indexing_done"`
	IndexingTotal         int       `json:"indexing_total"`
	LastFederationSuccess time.Time `json:"last_federation_success,omitempty"`
	LastFederationError   string    `json:"last_federation_error,omitempty"`
	Message               string    `json:"message"`
	Error                 string    `json:"error,omitempty"`
}

// Service is the single source of truth for node status.
type Service struct {
	mu        sync.RWMutex
	snap      Snapshot
	listeners map[int]chan Snapshot
	nextID    int
}

func New() *Service {
	return &Service{
		snap:      Snapshot{State: StateStarting, Message: "Starting"},
		listeners: make(map[int]chan Snapshot),
	}
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// SetState changes the lifecycle state. Same-state updates are a no-op.
func (s *Service) SetState(next State) error {
	return s.Update(func(snap *Snapshot) error {
		if snap.State == next {
			return nil
		}
		if !canTransition(snap.State, next) {
			return fmt.Errorf("invalid status transition %s → %s", snap.State, next)
		}
		snap.State = next
		if next != StateError {
			snap.Error = ""
		}
		return nil
	})
}

func (s *Service) SetError(userMessage string) error {
	return s.Update(func(snap *Snapshot) error {
		if !canTransition(snap.State, StateError) && snap.State != StateError {
			return fmt.Errorf("invalid status transition %s → error", snap.State)
		}
		snap.State = StateError
		snap.Error = userMessage
		snap.Message = userMessage
		return nil
	})
}

// Update applies fn to a copy of the snapshot and publishes the result.
func (s *Service) Update(fn func(*Snapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.snap
	if err := fn(&next); err != nil {
		return err
	}
	s.snap = next
	s.broadcast(next)
	return nil
}

func (s *Service) broadcast(snap Snapshot) {
	for id, ch := range s.listeners {
		select {
		case ch <- snap:
		default:
			// Drop stale snapshots so a slow tray/UI cannot block the node.
			_ = id
		}
	}
}

// Watch returns a channel of snapshots and an unsubscribe function.
// Opening a watcher must never perform network I/O.
func (s *Service) Watch() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 1)
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.listeners[id] = ch
	current := s.snap
	s.mu.Unlock()

	select {
	case ch <- current:
	default:
	}

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.listeners, id)
			s.mu.Unlock()
			close(ch)
		})
	}
	return ch, unsub
}
