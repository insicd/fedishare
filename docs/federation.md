# Federation (Phase 5)

FediShare speaks ActivityPub for identity and metadata only. File bytes never go through the inbox or outbox.

Every Actor publishes two fixed Mastodon-style profile fields (`attachment` PropertyValue): `My Web` with the actor’s public browser URL, then `Fedishare` with `https://github.com/insicd/fedishare`. The bio (`summary`) stays user-editable; those fields do not.

Public file posts are `Create`/`Update`/`Delete` of a `Note` with a `Document` attachment. The Note id is a dereferenceable HTTP URL (`/users/{username}/notes/{id}`), `content` is HTML, and both the activity and the Note address `as:Public` in `to` plus the local followers collection in `cc`. That matches what Mastodon, Friendica, and WAFRN expect when they refetch the object.

Browsers that open the actor or note URL with `Accept: text/html` receive a FediShare HTML page (bio, My Web and Fedishare fields, file list or download). Requests that prefer `application/activity+json` still get the ActivityPub document. The gateway stores only the JSON Actor in its offline cache.

Lemmy is different: it stores community `Page`/`Article` objects, and treats a bare `Note` as a comment that needs `inReplyTo` plus a community `audience`. Following a FediShare Person on Lemmy can show the profile without ever listing the file posts. FediShare does not post into Lemmy communities.

## HTTP Signatures

The MVP implements **draft-cavage-http-signatures** with `rsa-sha256`. This is what Mastodon, Pleroma, and Misskey expect today.

Outbound Actor GETs are signed the same way so instances that enable Mastodon **authorized fetch** / secure mode (including mastodon.social) return the public key instead of HTTP 401. Without that key a Follow cannot be verified and stays pending.

RFC 9421 HTTP Message Signatures is **not** implemented. If a request sends both a Cavage `Signature` and an RFC 9421 `Signature`, the Cavage header is used.

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
