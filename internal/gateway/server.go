package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/crypto"
	"github.com/fedishare/fedishare/internal/tunnel"
	"github.com/fedishare/fedishare/internal/version"
)

var usernameRE = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

const (
	maxInboxBody = 1 << 20
	maxCacheBody = 256 << 10
	maxPending   = 32
)

// Server is the public VPS process. It stores public keys and cached
// Actor/WebFinger documents only. It never stores private keys or files.
type Server struct {
	PublicURL string
	Store     *Store
	Log       *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

func New(store *Store, publicURL string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		PublicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"),
		Store:     store,
		Log:       log,
		sessions:  make(map[string]*session),
	}
}

// Handler is the public HTTP surface. The desktop admin UI is not here.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /v1/tunnel", s.upgrade)
	mux.HandleFunc("/", s.public)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"service":"fedishare-gateway","version":"` + version.Version + `"}`))
}

func (s *Server) upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := tunnel.AcceptUpgrade(w, r)
	if err != nil {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	go s.handshake(conn)
}

func (s *Server) handshake(conn *tunnel.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	typ, payload, err := conn.ReadFrame()
	if err != nil || typ != tunnel.TypeHello {
		return
	}
	var hello tunnel.Hello
	if err := json.Unmarshal(payload, &hello); err != nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(hello.Username))
	if !usernameRE.MatchString(username) {
		s.failAuth(conn, "invalid username")
		return
	}
	pub, err := crypto.ParsePublicKeyPEM([]byte(hello.PublicKey))
	if err != nil {
		s.failAuth(conn, "invalid public key")
		return
	}
	ch, err := tunnel.NewChallenge()
	if err != nil {
		s.failAuth(conn, "could not issue challenge")
		return
	}
	body, _ := json.Marshal(ch)
	if err := conn.WriteFrame(tunnel.TypeChallenge, body); err != nil {
		return
	}
	typ, payload, err = conn.ReadFrame()
	if err != nil || typ != tunnel.TypeAuth {
		return
	}
	var auth tunnel.Auth
	if err := json.Unmarshal(payload, &auth); err != nil {
		s.failAuth(conn, "invalid auth")
		return
	}
	if err := tunnel.CheckIssuedAt(ch.IssuedAt, time.Now().UTC()); err != nil {
		s.failAuth(conn, err.Error())
		return
	}
	if err := tunnel.VerifyChallenge(pub, username, ch.Nonce, ch.IssuedAt, auth.Signature); err != nil {
		s.failAuth(conn, "challenge signature invalid")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := s.Store.ConsumeNonce(ctx, ch.Nonce, time.Now().Add(10*time.Minute)); err != nil {
		cancel()
		s.failAuth(conn, "challenge replayed")
		return
	}
	if err := s.Store.Register(ctx, username, hello.PublicKey, hello.NodeID); err != nil {
		cancel()
		if errors.Is(err, ErrUsernameTaken) {
			s.failAuth(conn, ErrUsernameTaken.Error())
			return
		}
		s.failAuth(conn, "registration failed")
		return
	}
	cancel()
	ok, _ := json.Marshal(tunnel.AuthOK{Username: username})
	if err := conn.WriteFrame(tunnel.TypeAuthOK, ok); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	sess := newSession(username, conn)
	s.attach(sess)
	defer s.detach(sess)
	s.Log.Info("tunnel connected", "username", username)
	sess.readLoop(s.Log)
	s.Log.Info("tunnel disconnected", "username", username)
}

func (s *Server) failAuth(conn *tunnel.Conn, msg string) {
	body, _ := json.Marshal(tunnel.AuthFail{Error: msg})
	_ = conn.WriteFrame(tunnel.TypeAuthFail, body)
}

func (s *Server) attach(sess *session) {
	s.mu.Lock()
	if old, ok := s.sessions[sess.username]; ok {
		old.close()
	}
	s.sessions[sess.username] = sess
	s.mu.Unlock()
}

func (s *Server) detach(sess *session) {
	s.mu.Lock()
	if cur, ok := s.sessions[sess.username]; ok && cur == sess {
		delete(s.sessions, sess.username)
	}
	s.mu.Unlock()
	sess.close()
}

func (s *Server) lookup(username string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[strings.ToLower(strings.TrimSpace(username))]
}

func (s *Server) public(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "FediShare gateway %s\n", version.Version)
		return
	}
	username, kind, ok := classify(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if sess := s.lookup(username); sess != nil {
		var rw http.ResponseWriter = w
		var cap *capture
		if kind == kindActor || kind == kindWebFinger {
			cap = &capture{ResponseWriter: w, limit: maxCacheBody}
			rw = cap
		}
		if err := sess.proxy(r.Context(), rw, r); err != nil {
			if cap == nil || !cap.wrote {
				s.offline(w, r, username, kind)
			}
			return
		}
		if cap != nil && cap.code == http.StatusOK && cap.buf.Len() > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if kind == kindActor {
				_ = s.Store.SaveActor(ctx, username, cap.buf.String())
			} else {
				_ = s.Store.SaveWebFinger(ctx, username, cap.buf.String())
			}
			cancel()
		}
		return
	}
	s.offline(w, r, username, kind)
}

func (s *Server) offline(w http.ResponseWriter, r *http.Request, username string, kind pathKind) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	row, err := s.Store.Get(ctx, username)
	if err == nil {
		switch kind {
		case kindActor:
			if row.ActorJSON != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte(row.ActorJSON))
				}
				return
			}
		case kindWebFinger:
			if row.WebFingerJSON != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				w.Header().Set("Content-Type", "application/jrd+json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write([]byte(row.WebFingerJSON))
				}
				return
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Retry-After", "30")
	http.Error(w, "The FediShare node is offline.", http.StatusServiceUnavailable)
}

type pathKind int

const (
	kindOther pathKind = iota
	kindActor
	kindWebFinger
	kindFile
)

func classify(r *http.Request) (username string, kind pathKind, ok bool) {
	path := r.URL.Path
	if strings.Contains(path, "..") {
		return "", 0, false
	}
	if path == "/.well-known/webfinger" {
		user := webfingerUsername(r.URL.Query().Get("resource"))
		if user == "" {
			return "", 0, false
		}
		return user, kindWebFinger, true
	}
	user, rest, ok := splitUser(path)
	if !ok {
		return "", 0, false
	}
	if rest == "" {
		return user, kindActor, true
	}
	if strings.HasPrefix(rest, "/download/") {
		return user, kindFile, true
	}
	return user, kindOther, true
}

func splitUser(path string) (username, rest string, ok bool) {
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "users/") {
		return "", "", false
	}
	tail := strings.TrimPrefix(path, "users/")
	user, extra, found := strings.Cut(tail, "/")
	user = strings.ToLower(user)
	if !usernameRE.MatchString(user) {
		return "", "", false
	}
	if found {
		return user, "/" + extra, true
	}
	return user, "", true
}

func webfingerUsername(resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" || len(resource) > 512 {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(resource), "acct:") {
		rest := resource[5:]
		user, _, _ := strings.Cut(rest, "@")
		user = strings.ToLower(strings.TrimSpace(user))
		if usernameRE.MatchString(user) {
			return user
		}
		return ""
	}
	u, err := url.Parse(resource)
	if err != nil {
		return ""
	}
	user, _, ok := splitUser(u.Path)
	if !ok {
		return ""
	}
	return user
}

type capture struct {
	http.ResponseWriter
	buf   bytes.Buffer
	code  int
	limit int
	wrote bool
}

func (c *capture) WriteHeader(code int) {
	c.code = code
	c.wrote = true
	c.ResponseWriter.WriteHeader(code)
}

func (c *capture) Write(p []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	remain := c.limit - c.buf.Len()
	if remain > 0 {
		if len(p) > remain {
			c.buf.Write(p[:remain])
		} else {
			c.buf.Write(p)
		}
	}
	return c.ResponseWriter.Write(p)
}

func (c *capture) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type session struct {
	username string
	conn     *tunnel.Conn
	mu       sync.Mutex
	pending  map[string]*pending
	closed   bool
}

type pending struct {
	w     http.ResponseWriter
	wrote bool
	errc  chan error
}

func newSession(username string, conn *tunnel.Conn) *session {
	return &session{
		username: username,
		conn:     conn,
		pending:  make(map[string]*pending),
	}
}

func (sess *session) close() {
	sess.mu.Lock()
	if sess.closed {
		sess.mu.Unlock()
		return
	}
	sess.closed = true
	for id, p := range sess.pending {
		select {
		case p.errc <- errOffline:
		default:
		}
		delete(sess.pending, id)
	}
	sess.mu.Unlock()
	_ = sess.conn.Close()
}

var errOffline = errors.New("node offline")

func (sess *session) proxy(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxInboxBody+1))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return err
	}
	if len(body) > maxInboxBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return fmt.Errorf("body too large")
	}
	id, err := newReqID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return err
	}
	p := &pending{w: w, errc: make(chan error, 1)}
	sess.mu.Lock()
	if sess.closed || len(sess.pending) >= maxPending {
		sess.mu.Unlock()
		return errOffline
	}
	sess.pending[id] = p
	sess.mu.Unlock()
	defer func() {
		sess.mu.Lock()
		delete(sess.pending, id)
		sess.mu.Unlock()
	}()

	hdr := tunnel.StripHop(r.Header)
	if r.Host != "" {
		hdr.Set("Host", r.Host)
	}
	req := tunnel.HTTPReq{
		ID:     id,
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Header: hdr,
		Body:   body,
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := sess.conn.WriteFrame(tunnel.TypeHTTPReq, payload); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-p.errc:
		if err != nil && p.wrote {
			return nil
		}
		return err
	}
}

func (sess *session) readLoop(log *slog.Logger) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if err := sess.conn.WriteFrame(tunnel.TypePing, nil); err != nil {
				return
			}
		}
	}()
	for {
		typ, payload, err := sess.conn.ReadFrame()
		if err != nil {
			return
		}
		switch typ {
		case tunnel.TypePing:
			_ = sess.conn.WriteFrame(tunnel.TypePong, nil)
		case tunnel.TypePong:
		case tunnel.TypeHTTPResHead:
			var head tunnel.HTTPResHead
			if err := json.Unmarshal(payload, &head); err != nil {
				continue
			}
			sess.onHead(head)
		case tunnel.TypeHTTPResBody:
			id, data, err := decodeBody(payload)
			if err != nil {
				continue
			}
			sess.onBody(id, data)
		case tunnel.TypeHTTPResEnd:
			var end tunnel.HTTPResEnd
			if err := json.Unmarshal(payload, &end); err != nil {
				continue
			}
			sess.onEnd(end.ID)
		default:
			if log != nil {
				log.Debug("ignored tunnel frame", "type", typ)
			}
		}
	}
}

func (sess *session) onHead(head tunnel.HTTPResHead) {
	sess.mu.Lock()
	p := sess.pending[head.ID]
	sess.mu.Unlock()
	if p == nil {
		return
	}
	for k, vs := range tunnel.StripHop(http.Header(head.Header)) {
		for _, v := range vs {
			p.w.Header().Add(k, v)
		}
	}
	if head.Status == 0 {
		head.Status = http.StatusOK
	}
	p.w.WriteHeader(head.Status)
	p.wrote = true
}

func (sess *session) onBody(id string, data []byte) {
	sess.mu.Lock()
	p := sess.pending[id]
	sess.mu.Unlock()
	if p == nil {
		return
	}
	if !p.wrote {
		p.w.WriteHeader(http.StatusOK)
		p.wrote = true
	}
	_, _ = p.w.Write(data)
	if f, ok := p.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (sess *session) onEnd(id string) {
	sess.mu.Lock()
	p := sess.pending[id]
	sess.mu.Unlock()
	if p == nil {
		return
	}
	select {
	case p.errc <- nil:
	default:
	}
}

func decodeBody(payload []byte) (string, []byte, error) {
	if len(payload) < 16 {
		return "", nil, fmt.Errorf("short body frame")
	}
	return string(payload[:16]), payload[16:], nil
}

func newReqID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// HTTPServer builds the public listener. HTTP/2 is disabled so the
// HTTP/1.1 Upgrade tunnel can hijack the connection.
func HTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
}

// Serve accepts public HTTP on ln until it is closed.
func (s *Server) Serve(ln net.Listener) error {
	return HTTPServer(s.Handler()).Serve(ln)
}
