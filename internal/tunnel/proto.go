// Package tunnel is the reverse-channel protocol between a desktop node
// and a FediShare gateway.
//
// Transport is HTTP/1.1 Upgrade to "fedishare-tunnel" followed by
// length-prefixed frames. WebSocket, HTTP/2, and QUIC were considered;
// Upgrade on the existing HTTPS port is enough for NAT traversal, stays
// in the standard library, and avoids a generic VPN.
package tunnel

const (
	UpgradeProtocol = "fedishare-tunnel"
	ChallengeDomain = "fedishare-tunnel-v1"

	TypeHello       byte = 1
	TypeChallenge   byte = 2
	TypeAuth        byte = 3
	TypeAuthOK      byte = 4
	TypeAuthFail    byte = 5
	TypePing        byte = 6
	TypePong        byte = 7
	TypeHTTPReq     byte = 8
	TypeHTTPResHead byte = 9
	TypeHTTPResBody byte = 10
	TypeHTTPResEnd  byte = 11

	MaxFrame = 8 << 20
	Chunk    = 64 << 10
)

// Hello is the node's first message. It never includes a private key.
type Hello struct {
	Username    string `json:"username"`
	PublicKey   string `json:"public_key_pem"`
	NodeID      string `json:"node_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// Challenge is a one-time nonce the node must sign.
type Challenge struct {
	Nonce    string `json:"nonce"`
	IssuedAt string `json:"issued_at"`
}

// Auth is the node's signature of the challenge.
type Auth struct {
	Signature string `json:"signature"`
}

// AuthOK confirms the tunnel is bound to username.
type AuthOK struct {
	Username string `json:"username"`
}

// AuthFail explains a rejected handshake.
type AuthFail struct {
	Error string `json:"error"`
}

// HTTPReq is one proxied request from the gateway to the node.
type HTTPReq struct {
	ID     string              `json:"id"`
	Method string              `json:"method"`
	Path   string              `json:"path"`
	Query  string              `json:"query,omitempty"`
	Header map[string][]string `json:"header"`
	Body   []byte              `json:"body,omitempty"`
}

// HTTPResHead starts a proxied response.
type HTTPResHead struct {
	ID     string              `json:"id"`
	Status int                 `json:"status"`
	Header map[string][]string `json:"header"`
}

// HTTPResEnd finishes a proxied response.
type HTTPResEnd struct {
	ID string `json:"id"`
}
