# FediShare architecture (Phase 6)

This document records decisions that are already in the code.

## What Phase 6 implements

```
cmd/fedishare                    cmd/fedishare-gateway
    ├── tray (CGO)                    └── gateway.Server
    └── node.Node                         ├── actors + used_nonces (SQLite)
            ├── config / sqlite           ├── HTTP/1.1 Upgrade tunnel
            ├── indexer + downloads       └── public AP + download proxy
            ├── activitypub + federation
            ├── loopback dashboard
            └── tunnel.Client  ──────────►  outbound reverse tunnel
```

ActivityPub object `id` values use `gateway_url` (for example `https://nodes.example.org/users/alice`). The desktop still binds the admin UI to loopback. Public discovery and downloads reach the node through the gateway tunnel. The same actor and note URLs negotiate `Accept`: browsers get a public HTML profile; federation clients get JSON.

The gateway never stores private keys or file bytes. Local indexing works with `gateway_url` empty; the node then stays Offline and serves Actor/WebFinger only on loopback.

On Cloudron the same binary runs from the published Docker image. The platform terminates TLS and mounts `/app/data`. See [cloudron.md](cloudron.md).

Signing, Follow processing, and the delivery queue are documented in [federation.md](federation.md). The tunnel protocol is documented in [gateway.md](gateway.md).

Create activities wrap a Note (human-readable, Mastodon-compatible) with a Document attachment (namespaced hash/size/filename). ActivityPub is never used as a binary transport; `/users/{username}/download/{id}` reuses the Phase 3 streaming handler, now reachable through the gateway.

## Earlier phases

Phase 5 added inbox Follow/Undo/Accept/Reject/Delete, draft-cavage HTTP Signatures, and a durable delivery queue.

Phase 3 added the SHA-256 index, fsnotify watcher, and secure `/files/{id}` downloads.

Symlink policy: **symlinks are rejected**. The share root must be a real directory. Indexing and downloads use `Lstat` and `O_NOFOLLOW` (on Unix) and re-check that the resolved path still sits inside the share root.

Downloads use opaque ids (`/files/3f98ab…`), never filesystem paths. Content identity is `fedishare:<algorithm>:<digest>` (SHA-256 today).

The admin UI stays on loopback. Public downloads use the same `DownloadHandler` via `httpserver.PublicHandler`.

Phase 2 added the tray, wizard, dashboard, and local RSA keys.

A process start does the following:

1. Resolve `--data-dir` or the OS default home.
2. Create `home/`, `home/logs/`, and `home/keys/` with mode `0700`.
3. Create `config.json` with defaults if it does not exist.
4. Generate `keys/actor.pem` if it does not exist.
5. Open `fedishare.db`, apply WAL/foreign-key pragmas, run migrations.
6. Load or create a 128-bit `node_id` in the `config` table.
7. Bind the dashboard to `127.0.0.1:<local_port>`.
8. If `gateway_url` is set, dial the gateway and move `Connecting → Online`.
9. Otherwise stay `Offline` with “Gateway not connected”.
10. Show the tray and optionally open the browser.

Changing the shared folder later must not change this `node_id`. ActivityPub key material lives under `home/keys/` with `0600` files and is never sent to a gateway.

## Status model

`internal/status` is the only source of truth. The tray and dashboard consume this service rather than inferring state.

States: `Starting`, `Indexing`, `Connecting`, `Online`, `Paused`, `Offline`, `Error`, `ShuttingDown`.

`Online` means the reverse tunnel is authenticated and the node is sharing. `ActivityPubActive` is true once the node is configured. `GatewayConnected` is true only while the tunnel is up.

Watchers are non-blocking. A slow UI cannot stall indexing or I/O.

## Storage layout

```
<home>/
  config.json       user settings, 0600
  fedishare.db      SQLite (WAL), 0600
  logs/fedishare.log
  keys/actor.pem    RSA-2048 private key, 0600
  keys/actor.pub.pem
```

Gateway data directory (separate machine):

```
<data-dir>/
  gateway.db        usernames, public keys, cached Actor/WebFinger
  gateway.log
```

Desktop SQLite has:

- `schema_migrations` — applied migration versions
- `config` — runtime key/value (`node_id`, start timestamps)
- `files` / `directories` — share index (no file bytes)
- `followers` / `following` / `blocks` — social graph
- `activities` — outgoing ActivityPub documents
- `federation_queue` — durable remote inbox deliveries
- `inbox_replay` — signature hashes for replay protection

## Minimum dependencies

| Need | Choice | Why |
| --- | --- | --- |
| SQLite | `modernc.org/sqlite` (requires Go 1.25+) | Pure Go, no CGO, works in tests and on `fedishare-gateway` |
| Logging | `log/slog` | Standard library |
| Config | `encoding/json` | Standard library, human-readable |
| FS watch | `fsnotify` | Recursive, debounced; hashing runs off the callback |
| Tray | `fyne.io/systray` | Maintained fork, DBus on Linux, CGO |
| Local UI | `net/http` + `html/template` | Loopback only |
| Tunnel | HTTP/1.1 Upgrade + framed I/O | Standard library, outbound-only NAT traversal |

ActivityPub JSON and WebFinger JRD use the Go standard library. No ActivityPub framework is added.

`modernc.org/sqlite` is the only third-party module required by the gateway. The desktop binary still needs CGO for the tray.

## Platform-sensitive decisions for the tray (Phase 2)

These are recorded so later packaging work does not surprise us.

1. **Library.** A mature cgo-based systray (`fyne.io/systray`) is the practical option for a menu-bar/notification-area icon on Windows, macOS, and Linux.

2. **CGO.** The `fedishare` desktop binary requires a C compiler and, on Linux, GTK/AppIndicator development headers. The gateway binary stays CGO-free.

3. **CI cannot casually cross-compile the desktop app.** Phase 7 CI should compile `fedishare` on native runners, and may still cross-compile `fedishare-gateway` with `CGO_ENABLED=0`.

4. **macOS menu-bar app.** A `.app` bundle will need `LSUIElement=1` so FediShare lives in the status bar without a Dock icon.

5. **Windows notification area.** Production Windows builds should use `-H windowsgui`.

6. **Linux trays.** StatusNotifier / AppIndicator works on GNOME (via an extension), KDE, and most Ubuntu setups. Environments without a tray should still run the daemon and the local web UI.

7. **Icons.** Tray state variants live under `assets/` and must not include third-party copyrighted images.

8. **Hidden console vs. logs.** File logging is the support channel once the tray exists.

9. **Permissions.** `chmod 0600/0700` is meaningful on macOS and Linux. `crypto.KeyStore` is the seam for a later OS keychain.

10. **Single-instance and autostart.** These are Phase 7. `--data-dir` is already the hook for running two isolated nodes on one machine during federation testing.
