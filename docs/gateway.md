# Gateway (Phase 6)

The gateway is a small Go process that runs on a public VPS. It gives a desktop node behind NAT a stable Actor URL. It is not a file host and it is not required for local indexing.

The desktop defaults `gateway_url` to `https://fedishare.console.itagora.it`. Operators who run their own `fedishare-gateway` can replace that URL in Settings.

## What it stores

| Stored | Not stored |
| --- | --- |
| Username | Private keys |
| Public key PEM | File bytes |
| Cached Actor JSON | Admin session / CSRF |
| Cached WebFinger JRD | Share-folder paths |
| Used challenge nonces | |

The first public key that registers a username wins. A later node that presents a different key for the same name is rejected.

`GET /.well-known/fedishare-network` lists registered usernames (name, public URL, online if a tunnel is up). It is how the desktop **FediShare network** page finds people on the same gateway. Other gateways are not linked automatically: ActivityPub has no global user directory. A user can paste another gateway’s URL or an `@user@host` address. The document does not include public keys, private keys, or file bytes.

## Transport

The desktop node opens an **outbound** HTTP/1.1 connection and upgrades it to `fedishare-tunnel`. The gateway never needs a port-forward on the user's machine.

| Option | Decision |
| --- | --- |
| WebSocket | Rejected — extra dependency, no benefit over Upgrade |
| HTTP/2 | Rejected — awkward to reverse-stream; HTTP/2 is disabled (`TLSNextProto` empty) so Upgrade can hijack |
| QUIC | Rejected — UDP, extra stack, harder to operate on a cheap VPS |
| HTTP/1.1 Upgrade | Chosen — stdlib only, one TCP/TLS port, framed reverse HTTP |

Frames are `uint32be length + uint8 type + payload`. Downloads are streamed as 64 KiB `http_res` body frames. Responses are never buffered in an `httptest.ResponseRecorder`.

## Authentication

1. Node sends `hello` (username + public key PEM + node id).
2. Gateway replies with a random nonce and `issued_at`.
3. Node signs `SHA-256("fedishare-tunnel-v1\n" + username + "\n" + nonce + "\n" + issued_at)` with RSA-PKCS1v15.
4. Gateway verifies the signature against the hello public key, then against the registered key.
5. Nonces are persisted and cannot be reused. `issued_at` must be within two minutes.

The private key never leaves `keys/actor.pem` on the desktop.

## Offline behaviour

When the tunnel is down:

- File downloads and inbox POST return **503** with `Retry-After: 30`
- Actor and WebFinger are served from cache when a previous online response was stored
- The desktop UI shows **Gateway disconnected** (or **Connecting** while retrying)

The node reconnects with bounded exponential backoff (1s → 60s) plus jitter.

## Public paths

The gateway reverse-proxies only:

```
GET  /.well-known/webfinger
GET  /.well-known/fedishare-network
GET  /users/{username}
GET  /users/{username}/outbox
GET  /users/{username}/followers
GET  /users/{username}/following
GET  /users/{username}/files/{id}
GET  /users/{username}/activities/{id}
GET  /users/{username}/download/{id}
POST /users/{username}/inbox
```

`/`, `/api/`, `/static/`, and the loopback dashboard are never exposed. The admin UI stays on `127.0.0.1`.

## Run

```bash
CGO_ENABLED=0 go build -o bin/fedishare-gateway ./cmd/fedishare-gateway

./bin/fedishare-gateway \
  --listen 0.0.0.0:443 \
  --data-dir /var/lib/fedishare-gateway \
  --public-url https://nodes.example.org \
  --tls-cert /etc/ssl/nodes.example.org.crt \
  --tls-key /etc/ssl/nodes.example.org.key
```

On the desktop, set `gateway_url` to that public origin. The node dials `/v1/tunnel` and, once authenticated, moves to **Online**.

## Cloudron

To run the gateway on Cloudron (HTTPS and the domain come from the platform), see [cloudron.md](cloudron.md). The container listens on HTTP `:8000` and stores `gateway.db` in `/app/data`.
