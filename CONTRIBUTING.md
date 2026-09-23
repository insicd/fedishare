# Contributing to FediShare

Thank you for considering a contribution.

## Development setup

Install Go 1.26 or newer. The desktop tray needs CGO (the default on macOS). Use `CGO_ENABLED=0` or `--no-tray` for headless tests.

```bash
go test ./...
go vet ./...
gofmt -l .
go build -o bin/fedishare ./cmd/fedishare
CGO_ENABLED=0 go build -o bin/fedishare-gateway ./cmd/fedishare-gateway
docker build --platform linux/amd64 -t fedishare-gateway:local .
```

Please run `gofmt` on every Go file you touch. Do not submit code that fails `go test` or `go vet`.

## Project conventions

- Prefer the Go standard library.
- Add a dependency only when it materially improves correctness or portability, and mention it in the README.
- Keep ActivityPub, file indexing, HTTP file transfer, and the gateway tunnel in separate packages.
- Do not commit secrets, private keys, or real shared-folder contents.
- Logs must not include private keys, tokens, or full remote IP addresses.

The application is built in numbered phases. Please do not land Phase N+1 work while Phase N still fails to compile or test.

## Pull requests

1. Keep changes focused.
2. Include tests for logic that can break (validation, state transitions, migrations, path safety).
3. Update the README when user-visible behavior changes.
4. Add a `CHANGELOG.md` note.

## Reporting security issues

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
