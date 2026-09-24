# Federation (Phase 5)

FediShare speaks ActivityPub for identity and metadata only. File bytes never go through the inbox or outbox.

Every Actor publishes a fixed Mastodon-style profile field (`attachment` PropertyValue) named `Fedishare` with value `https://github.com/insicd/fedishare`. The bio (`summary`) stays user-editable; that field does not.

## HTTP Signatures

The MVP implements **draft-cavage-http-signatures** with `rsa-sha256`. This is what Mastodon, Pleroma, and Misskey expect today.

RFC 9421 HTTP Message Signatures is **not** implemented.

Signed POST requests include:

```
(request-target)
host
date
digest
```

`Digest` is `SHA-256=` plus Base64 of the SHA-256 of the exact body bytes.

`Date` must be within 30 seconds in the future and 12 hours in the past. The hex SHA-256 of the signature value is stored for 24 hours so the same signature cannot be replayed.

Unsigned inbox POSTs return 401. Verification is never disabled to make tests pass; tests sign real keys.

## Inbox

`POST /users/{username}/inbox` accepts:

| Type | Effect |
| --- | --- |
| Follow | Store follower, auto-Accept, queue Accept to their inbox |
| Undo (Follow) | Remove follower |
| Accept / Reject | Update an outgoing follow we previously recorded |
| Delete | Remove the follower if they deleted their actor |

Other types are ignored (202 is not returned for invalid JSON or bad signatures).

Body limit: 1 MiB.

## Delivery queue

Index changes enqueue Create, Update, or Delete activities. The worker POSTs them to each accepted follower inbox (sharedInbox when present, de-duplicated).

- Temporary failures (network, 408, 429, 5xx): exponential backoff, max 12 attempts, cap 1 hour
- Permanent failures (400, 401, 403, 404, 410, 422): marked failed, not retried
- Queue rows survive process restart

Pause sharing does not enqueue new file activities and skips file deliveries already in the queue. Accept of a Follow is still delivered.

## SSRF

Outbound federation HTTP (actor fetch and delivery) refuses:

- loopback, RFC1918, link-local, multicast
- `169.254.169.254` and `*.internal` metadata hosts
- non-http(s) schemes

Two local `--data-dir` nodes on one machine, or tests, set `node.Options.AllowLocalFederation` (or leave `gateway_url` empty) so loopback fetches are allowed.

## Reachability

The desktop still binds the admin UI to `127.0.0.1`. Public WebFinger, Actor, inbox, and downloads are reverse-proxied by `fedishare-gateway` over the authenticated outbound tunnel. See [gateway.md](gateway.md).
