# FediShare gateway image for Cloudron and generic Docker hosts.
# Build for Cloudron (linux/amd64):
#   docker build --platform linux/amd64 -t YOURUSER/fedishare-gateway:0.6.0 .
#
# The runtime stage is cloudron/base (amd64). Do not pass --tls-cert:
# Cloudron (or another reverse proxy) terminates HTTPS.

FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath -ldflags="-s -w" -o /out/fedishare-gateway ./cmd/fedishare-gateway

FROM cloudron/base:5.0.0@sha256:04fd70dbd8ad6149c19de39e35718e024417c3e01dc9c6637eaf4a41ec4e596c

RUN mkdir -p /app/code
WORKDIR /app/code

COPY --from=build /out/fedishare-gateway /app/code/fedishare-gateway
COPY packaging/cloudron/start.sh /app/code/start.sh
RUN chmod 0755 /app/code/fedishare-gateway /app/code/start.sh

EXPOSE 8000

CMD ["/app/code/start.sh"]
