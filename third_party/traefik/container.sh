#!/usr/bin/env bash
set -euo pipefail

# This script mirrors third_party/traefik/docker-compose.yaml using Apple's "container" CLI
# Service: Traefik v2.10
# Name: traefik

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NAME="traefik"
IMAGE="traefik:v2.10"

MOUNTS=(
  "--mount" "type=bind,src=${SCRIPT_DIR},dst=/etc/traefik,ro=true"
)

# Run Traefik
container run -d \
  --name "${NAME}" \
  --publish 8081:80 \
  --publish 8080:8080 \
  "${MOUNTS[@]}" \
  "${IMAGE}" \
  --api.dashboard=true \
  --api.insecure=true \
  --providers.file.filename=/etc/traefik/dynamic.yml \
  --entrypoints.web.address=:80 \
  --entrypoints.websecure.address=:443 \
  --log.level=DEBUG \
  --accesslog=true

echo "Traefik is starting as container '${NAME}'. Logs:" >&2
container logs -f "${NAME}"
