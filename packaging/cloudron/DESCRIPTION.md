FediShare Gateway gives desktop FediShare nodes a stable ActivityPub address.

It is a rendezvous service, not a file host. The container stores usernames, public keys, and a small Actor/WebFinger cache under `/app/data`. Private keys and shared files stay on the user's computer.

After install, set `gateway_url` on the desktop app to this location's HTTPS origin (for example `https://nodes.example.org`).
