#!/bin/bash
set -eu

# Cloudron: writable data is only /app/data, /tmp, and /run.
# TLS terminates at the platform proxy. Do not pass --tls-cert here.

mkdir -p /app/data

export FEDISHARE_LISTEN="${FEDISHARE_LISTEN:-0.0.0.0:8000}"
export FEDISHARE_DATA_DIR="${FEDISHARE_DATA_DIR:-/app/data}"
export FEDISHARE_LOG_LEVEL="${FEDISHARE_LOG_LEVEL:-info}"

if [ -z "${FEDISHARE_PUBLIC_URL:-}" ] && [ -n "${CLOUDRON_APP_ORIGIN:-}" ]; then
	export FEDISHARE_PUBLIC_URL="${CLOUDRON_APP_ORIGIN}"
fi

exec /app/code/fedishare-gateway \
	--listen "${FEDISHARE_LISTEN}" \
	--data-dir "${FEDISHARE_DATA_DIR}" \
	--public-url "${FEDISHARE_PUBLIC_URL:-}" \
	--log-level "${FEDISHARE_LOG_LEVEL}"
