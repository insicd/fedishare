# Security policy

FediShare shares files from a directory you choose. Treat every remote ActivityPub server, every gateway, and every download client as untrusted.

## Reporting a vulnerability

Please email the maintainers privately rather than opening a public GitHub issue. Include:

- a description of the issue
- steps to reproduce
- the FediShare version / commit
- the impact (file exposure, identity takeover, denial of service, …)

We will acknowledge the report and work on a fix before any public disclosure.

## Phase 6 scope

The gateway listens on a public address. Treat it as an untrusted relay:

- it never receives or stores the actor private key
- it never hosts or mirrors file bytes
- the first registered public key owns a username
- tunnel auth is challenge-response with one-time nonces and a 2-minute `issued_at` window
- only ActivityPub and `/users/{username}/download/{id}` are proxied; `/api` and the dashboard are not
- HTTP/2 is disabled on the gateway so the HTTP/1.1 Upgrade cannot be skipped
- offline nodes return 503 for file content; Actor/WebFinger may be served from a small cache
- the desktop reconnects with bounded backoff; a hostile gateway still cannot sign as the actor

## Phase 5 scope

Phase 5 still listens only on `127.0.0.1`. Remote servers cannot complete WebFinger or inbox POST until the gateway exists.

Additional mitigations:

- inbox POST requires a valid HTTP Signature; unsigned bodies are 401
- Date skew and signature-hash replay protection
- inbox body capped at 1 MiB
- remote actor fetch and delivery use an SSRF policy (no loopback, private, link-local, or cloud metadata)
- blocked actors/domains are refused
- CSRF is skipped only for `POST /users/{username}/inbox`, not for the admin API
- WebFinger `resource` is length-limited and must be `acct:` or the actor URL
- only this node's username is served; other names return 404
- outbox and Document objects include only `available` files with `visibility=public`
- the Actor publishes the PEM **public** key only

## Phase 2–3 scope

Phase 2 listens only on `127.0.0.1` for the dashboard. It does not federate or serve shared files yet. The local risk surface is:

- the application data directory (`config.json`, SQLite, future key files)
- log files

Mitigations already in place:

- data, log, and key directories are created with mode `0700`
- `config.json` and the database are written with mode `0600` where the OS allows it
- the private-key directory exists and is empty; keys will never be stored in SQLite or in logs
- slog redacts attributes named like `private_key`, `token`, `password`, and similar
- filesystem paths in logs have the home-directory prefix replaced with `~`
- classified errors expose a short UI message and keep the technical cause in logs
- the admin API rejects non-loopback Host headers (DNS rebinding)
- state-changing requests require a CSRF token and a same-origin Origin header
- request bodies are size-limited
- the share folder cannot be the data directory or a path that contains it
- directory listing refuses symlink roots and does not follow symlink entries
- downloads resolve an opaque id against SQLite, then re-check the canonical path
- `../`, encoded traversal, and symlink components are rejected before open
- Unix downloads open the file with `O_NOFOLLOW`
- only `visibility=public` is served (other modes exist in the schema but are denied)
- pause returns 503 for new downloads
- Range requests are parsed; the file is streamed, never loaded entirely into RAM

## Issues that later phases must treat as hostile

These are not implemented yet. They are listed now so they are not forgotten.

- **Malicious ActivityPub payloads** — untrusted JSON, unexpected types, huge graphs
- **Oversized inbox requests** — hard body-size limits and timeouts
- **Directory traversal and encoded paths** — never accept a filesystem path from HTTP
- **Symlinks** — the MVP will refuse to share or serve symlinks
- **Local filesystem exposure** — only the user-selected share root is eligible
- **MIME confusion** — serve a sniffed/stored type, not a client-supplied one
- **Download flooding / DoS** — connection, concurrency, and bandwidth limits
- **Federation spam** — follower blocking and domain blocklists
- **SSRF during federation** — do not fetch localhost, loopback, link-local, private ranges, or cloud metadata endpoints
- **DNS rebinding against the local UI** — bind the admin API to `127.0.0.1` and check origins
- **Gateway compromise / malicious gateways** — the gateway never receives the private key; it only routes after a signed challenge
- **Key compromise** — keys stay local; a later keystore interface can move them into the OS keychain

ActivityPub is not a file-transfer protocol. Do not send file bytes through the inbox or outbox.
