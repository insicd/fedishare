# FediShare

FediShare turns your computer into a small ActivityPub file-sharing node.

Metadata federates over ActivityPub. The files themselves stay on your computer. A lightweight gateway gives machines behind NAT a stable public address; it is not a file host.

This repository is at **Phase 6**: a local desktop daemon plus a CGO-free `fedishare-gateway` that authenticates an outbound reverse tunnel and publishes stable Actor URLs. Autostart and packaging are still Phase 7.

## What FediShare will do

1. You choose a username and a shared folder.
2. FediShare creates a local ActivityPub identity such as `@alice@fedishare.console.itagora.it`.
3. Files in that folder are indexed and published as ActivityStreams objects.
4. Remote Fediverse users can discover and follow the actor.
5. Downloads are streamed from your disk over HTTP, through an optional rendezvous/gateway.
6. ActivityPub is never used as a binary transport.

## Current status

| Area | Status |
| --- | --- |
| OS-specific data directory and `config.json` | Done — created on first run |
| Structured logging with secret/path redaction | Done |
| SQLite + numbered migrations | Done |
| Node start / graceful shutdown | Done |
| Status state machine | Done |
| System tray + local dashboard + wizard | Done |
| RSA identity keys (local only) | Done |
| Pause / resume sharing | Done — pause returns 503 on new downloads |
| File index, hashing, local `/files/{id}` | Done |
| ActivityPub actor, WebFinger, outbox | Done — served on loopback; public URLs use `gateway_url` |
| Inbox, HTTP signatures, delivery queue | Done — Follow/Undo/Accept/Reject/Delete; draft-cavage rsa-sha256 |
| Gateway + reverse tunnel | Done — HTTP/1.1 Upgrade, challenge-response, streamed downloads |
| Cloudron / Docker image | Done — `Dockerfile` + `CloudronManifest.json`; see [docs/cloudron.md](docs/cloudron.md) |
| Autostart, desktop packaging, CI | Phase 7 |

## Requirements

- Go 1.26 or newer
- A C compiler for the desktop tray (`CGO_ENABLED=1`, the default on macOS). Use `--no-tray` or `CGO_ENABLED=0` for headless tests.

## Build and run

```bash
go test ./...
go vet ./...
go build -o bin/fedishare ./cmd/fedishare
CGO_ENABLED=0 go build -o bin/fedishare-gateway ./cmd/fedishare-gateway

./bin/fedishare --version
./bin/fedishare --data-dir ./.fedishare-home --no-tray --no-open

./bin/fedishare-gateway --version
./bin/fedishare-gateway --listen 127.0.0.1:8080 --data-dir ./.fedishare-gateway --public-url http://127.0.0.1:8080
```

To publish the gateway for Cloudron (linux/amd64 image on Docker Hub):

```bash
docker build --platform linux/amd64 -t nuke86/fedishare-gateway:0.6.0 .
docker push nuke86/fedishare-gateway:0.6.0
cloudron install --image nuke86/fedishare-gateway:0.6.0 --location nodes
```

See [docs/cloudron.md](docs/cloudron.md).

On first start FediShare writes `config.json` into the data directory, generates `keys/actor.pem`, and opens the dashboard at `http://127.0.0.1:17890/`. If you have not chosen a username and folder yet, that page is the first-run wizard.

`--data-dir` is for development and for running more than one node on one machine later. Production builds use the OS application-data directory:

| OS | Directory |
| --- | --- |
| macOS | `~/Library/Application Support/FediShare` |
| Windows | `%AppData%\FediShare` |
| Linux | `$XDG_CONFIG_HOME/fedishare` or `~/.config/fedishare` |

The shared folder is never used as the config or database location.

One desktop app can run several **profiles**. Each profile is a separate Actor (`@alice@…`, `@work@…`) with its own shared folder and RSA key. They all use the same `gateway_url`. Existing single-actor data directories are moved into `profiles/` on first launch.

Useful flags:

| Flag | Meaning |
| --- | --- |
| `--data-dir` | Override the application data directory |
| `--log-level` | `debug`, `info`, `warn`, or `error` |
| `--no-tray` | Do not attach a menu-bar / notification-area icon |
| `--no-open` | Do not open the dashboard in a browser |
| `--version` | Print the version and exit |

## Configuration

`config.json` is created automatically on the first run with defaults. The wizard and Settings page update it. You should not need to edit it by hand.

```json
{
  "username": "alice",
  "display_name": "Alice",
  "summary": "",
  "share_directory": "/Users/alice/FediShare",
  "gateway_url": "https://fedishare.console.itagora.it",
  "local_port": 17890,
  "max_concurrent_downloads": 8,
  "bandwidth_limit_bps": 0,
  "log_level": "info",
  "start_at_login": false
}
```

The private key is stored as `keys/actor.pem` with mode `0600`. It is never sent to a gateway. The matching public key is published on the Actor document and used to authenticate the reverse tunnel.

Public ActivityPub endpoints (on the gateway, and also on loopback for local tests):

```
GET /.well-known/webfinger?resource=acct:alice@nodes.example.org
GET /users/alice
GET /users/alice/outbox
GET /users/alice/outbox?page=1
GET /users/alice/notes/{id}
GET /users/alice/files/{id}
GET /users/alice/download/{id}
```

JSON `id` fields use `gateway_url` when it is set. The desktop node dials that origin, upgrades to `fedishare-tunnel`, and signs a one-time challenge. See [docs/gateway.md](docs/gateway.md).

A browser that opens `/users/{username}` or `/users/{username}/notes/{id}` (with `Accept: text/html`) gets a FediShare HTML page. Fediverse servers that ask for `application/activity+json` still receive the ActivityPub document.

Inbox:

```
POST /users/alice/inbox
```

Incoming POSTs must be signed with HTTP Signatures (`draft-cavage`, `rsa-sha256`, Date + Digest). Unsigned or replayed signatures are rejected. Follows are auto-accepted; Create/Update/Delete for shared files are queued and delivered to each follower's inbox (or sharedInbox) with backoff. Pause stops new file publications and downloads; Accept of a Follow still goes out.

## Architecture

Layers stay independent:

```
ActivityPub     identity, discovery, metadata, federation
FediShare       file index, content hashes, availability
Transport       HTTP now, P2P later
Gateway         stable addressing and NAT traversal
```

See [docs/architecture.md](docs/architecture.md).

## Dependencies

- [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) — CGO-free SQLite driver
- [`fyne.io/systray`](https://github.com/fyne-io/systray) — system tray / menu bar (CGO)
- [`github.com/fsnotify/fsnotify`](https://github.com/fsnotify/fsnotify) — recursive folder watch

Everything else is the Go standard library.

## License

Apache License 2.0. See [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please read [SECURITY.md](SECURITY.md) before reporting a vulnerability.
