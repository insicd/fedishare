# Deploy the gateway on Cloudron

The gateway image is a Cloudron custom app. Cloudron owns the domain and TLS. The container listens for HTTP on port 8000 and writes `gateway.db` under `/app/data`.

Do not pass `--tls-cert` / `--tls-key` inside the container.

## What you need

- A Cloudron box and a location (for example `nodes.example.org`) already pointing at it
- The [Cloudron CLI](https://docs.cloudron.io/packaging/cli/): `npm install -g cloudron`
- Optional: a Docker Hub account, to publish the image instead of building on the box

Cloudron boxes are **linux/amd64**. On Apple Silicon always pass `--platform linux/amd64`.

## Community app from the Cloudron UI (“My”)

The dashboard does not take a Dockerfile. It wants a **`CloudronVersions.json`**: a small catalog that points at the Docker image you already pushed.

1. Open `CloudronVersions.json` in this repo.
2. Replace `nuke86/fedishare-gateway:0.6.0` with the exact Hub image you pushed (for example `alice/fedishare-gateway:0.6.0`).
3. In Cloudron: **App Store → Community apps / My → add** and upload that file  
   (or paste a public HTTPS URL to it, if you host the file).
4. Install **FediShare Gateway** on the domain you already attached.

The image must be **public** on Docker Hub, or Cloudron cannot pull it. The box is `linux/amd64`; the image tag you name in `dockerImage` must exist.

`iconUrl` / `mediaLinks` in that file point at GitHub raw URLs. If those 404 until you push this repo, Cloudron may still install as long as `dockerImage` pulls. You can later point them at any public HTTPS PNG.

## Option A — Docker Hub, then CLI install

Create a public repository on hub.docker.com (the free plan allows unlimited public repos). Then, from the FediShare source tree:

```bash
docker login

docker build --platform linux/amd64 \
  -t nuke86/fedishare-gateway:0.6.0 \
  -t nuke86/fedishare-gateway:latest \
  .

docker push nuke86/fedishare-gateway:0.6.0
docker push nuke86/fedishare-gateway:latest

cloudron login your.cloudron.host

# run this from the repository root so CloudronManifest.json is found
cloudron install \
  --image nuke86/fedishare-gateway:0.6.0 \
  --location nodes
```

`--location nodes` becomes `https://nodes.your-domain` (or the custom domain you attach in the Cloudron UI). Set that exact origin as `gateway_url` on the desktop node.

Updates:

```bash
docker build --platform linux/amd64 -t nuke86/fedishare-gateway:0.6.1 .
docker push nuke86/fedishare-gateway:0.6.1
cloudron update --image nuke86/fedishare-gateway:0.6.1
```

## Option B — build on the Cloudron box

No Hub account required. From the repository root:

```bash
cloudron login your.cloudron.host
cloudron install --location nodes
```

The CLI uploads the source (see `.dockerignore`) and builds the Dockerfile on the server.

## After install

1. Open `https://<location>/healthz` — you should see `{"ok":true,...}`.
2. In the FediShare desktop app, set **Gateway URL** to `https://<location>` (no trailing slash).
3. The tray / dashboard should move to **Connecting**, then **Online**.

The first public key that registers a username wins. The gateway never stores the private key or file bytes.

## Tunnel through nginx

The desktop uses HTTP/1.1 `Upgrade: fedishare-tunnel`. Cloudron's proxy already forwards `Upgrade` for WebSockets. If the node stays on **Connecting** after a healthy `/healthz`, the proxy is not passing that custom upgrade; say so and we can add a WebSocket transport.

Long downloads share the proxy's read timeout. The node pings the tunnel every 20 seconds so idle connections stay up.

## Generic Docker (not Cloudron)

```bash
docker run --rm \
  --platform linux/amd64 \
  -p 8080:8000 \
  -e FEDISHARE_PUBLIC_URL=https://nodes.example.org \
  -v fedishare-gateway:/app/data \
  nuke86/fedishare-gateway:0.6.0
```

Put a TLS terminator in front. The same volume must be kept across restarts: it holds registered usernames and public keys.
