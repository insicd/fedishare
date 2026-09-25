package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/fedishare/fedishare/internal/network"
)

func (s *Server) network(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := s.Store.List(ctx, networkMaxActors)
	if err != nil {
		http.Error(w, "directory unavailable", http.StatusInternalServerError)
		return
	}
	base := s.publicBase(r)
	online := s.onlineSet()
	actors := make([]network.Actor, 0, len(rows))
	for _, row := range rows {
		actors = append(actors, network.ActorFromDocument(row.Username, base, row.ActorJSON, online[row.Username]))
	}
	dir := network.Directory{
		Type:    network.TypeDirectory,
		Gateway: base,
		Actors:  actors,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=15")
	_ = json.NewEncoder(w).Encode(dir)
}

func (s *Server) publicBase(r *http.Request) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) onlineSet() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]bool, len(s.sessions))
	for user := range s.sessions {
		out[user] = true
	}
	return out
}

const networkMaxActors = 500
