# Changelog

All notable changes to FediShare are documented here.

## 0.6.0-dev — Phase 6

- Add `fedishare-gateway`: a CGO-free VPS process that registers usernames to public keys and reverse-proxies ActivityPub plus downloads.
- Open an outbound HTTP/1.1 Upgrade tunnel (`fedishare-tunnel`) with length-prefixed frames so nodes behind NAT need no port-forward.
- Authenticate the tunnel with a signed challenge (RSA-PKCS1v15 over a gateway nonce); first public key wins the username; nonces cannot be replayed.
- Stream file downloads through the tunnel in 64 KiB chunks; return 503 for content when the node is offline and serve a cached Actor/WebFinger.
- Move the desktop to Connecting → Online when the tunnel is up, with bounded exponential backoff and jitter on reconnect.
- Keep the admin dashboard on loopback; the gateway never exposes `/api` or stores private keys or file bytes.
- Package the gateway for Cloudron and Docker Hub (`Dockerfile`, `CloudronManifest.json`): HTTP on :8000, data in `/app/data`, TLS left to the platform.
- Keep the same file id (and download URL) when a shared file is renamed or moved; publish an Update instead of Delete+Create. Directory moves trigger a rescan so children are not left pointing at dead paths.
- Default the desktop `gateway_url` to `https://fedishare.console.itagora.it` (users can still point at their own gateway).
- Publish a fixed Actor profile field `Fedishare` → `https://github.com/insicd/fedishare` on every node; it is not user-editable.
- Serve each file Note at a dereferenceable `/users/{username}/notes/{id}` (no `#object` fragment), address followers in `cc`, and emit HTML `content` so Friendica and WAFRN can refetch and render posts. Lemmy still will not list these Notes as community posts.
- Serve a public FediShare HTML profile (and per-file note page) when a browser sends `Accept: text/html`; ActivityPub clients still receive JSON. The gateway caches only Actor JSON so an HTML visit cannot poison federation.

## 0.5.0-dev — Phase 5

- Accept signed `POST /users/{username}/inbox` for Follow, Undo Follow, Accept, Reject, and actor Delete.
- Verify and produce Mastodon-compatible HTTP Signatures (`draft-cavage`, rsa-sha256) with Date, Digest, and replay protection.
- Persist followers, blocks, outgoing activities, and a durable delivery queue with exponential backoff.
- Publish Create/Update/Delete to followers when the share index changes; pause skips file deliveries.
- Fetch remote actors behind an SSRF policy (no loopback/private/metadata unless local-federation testing).
- Dashboard Followers list with per-actor block.

## 0.4.0-dev — Phase 4

- Serve a standards-compliant ActivityPub Person at `/users/{username}` with inbox, outbox, followers, following, and a local RSA public key.
- Implement WebFinger at `/.well-known/webfinger` for `acct:user@gateway-host`.
- Publish an OrderedCollection outbox of Create activities (Note + Document attachment) generated from the public file index.
- Expose per-file Document and Create objects, and stream downloads at `/users/{username}/download/{id}`.
- Followers, inbox POST, HTTP signatures, and remote delivery remain Phase 5.

## 0.3.0-dev — Phase 3

- Index the shared folder in SQLite with opaque file ids and SHA-256 content identity (`fedishare:sha256:<digest>`).
- Watch the share tree with fsnotify (create/modify/rename/delete), debounce events, and skip files that are still being written.
- Serve local downloads at `/files/{id}` with GET/HEAD, Range, ETag, and streaming I/O.
- Reject symlinks and path traversal; pause stops new downloads with 503.
- Generate ActivityStreams Document objects for later federation (not published yet).

## 0.2.0-dev — Phase 2

- Write `config.json` on first run with defaults, then update it from the wizard and Settings.
- Add a loopback dashboard, first-run wizard, and CSRF/origin-protected local API.
- Add a system tray menu that follows the shared status service.
- Generate a local RSA-2048 actor key pair; the private key never leaves the machine.
- Add pause/resume and a symlink-safe shared-folder listing (no hashing yet).

## 0.1.0-dev — Phase 1

- Initialize the Go module, Apache-2.0 license, and repository documentation.
- Add OS-specific data directories and a validated `config.json`.
- Add structured logging with secret and home-path redaction.
- Add SQLite via `modernc.org/sqlite`, WAL mode, and numbered SQL migrations.
- Add a node lifecycle that starts, records a stable installation ID, and shuts down cleanly.
- Add an explicit status state machine shared by future tray and dashboard code.
