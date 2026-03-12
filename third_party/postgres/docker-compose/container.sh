#!/usr/bin/env bash
set -euo pipefail

# This script mirrors docker-compose.yaml using Apple's open-source `container` CLI
# Service: Postgres 15.4 (single instance)
# Name: pg_local
# Data: Persistent via bind mount with permissive perms (avoid entrypoint chown)
# Init SQL: ./initdb -> /docker-entrypoint-initdb.d (executed on first start)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INIT_DIR="${SCRIPT_DIR}/initdb"
DATA_DIR="${SCRIPT_DIR}/pgdata"
NAME="postgres"
IMAGE="postgres:15.4"

# Prepare directories
mkdir -p "${INIT_DIR}"
mkdir -p "${DATA_DIR}"
# Relax perms so UID/GID 999 can write without chown

# Remove previous container if exists (ignore errors)
container rm -f "${NAME}" >/dev/null 2>&1 || true

# Run Postgres with PGDATA bind mount and postgres UID/GID (usually 999)
container run -d \
	--name "${NAME}" \
    --user 0:0 \
	--publish 5432:5432 \
	--mount type=bind,src="${INIT_DIR}",dst=/docker-entrypoint-initdb.d,ro=true \
	--env POSTGRES_USER=user \
	--env POSTGRES_PASSWORD=password \
	--env POSTGRES_DB=land_contract \
	"${IMAGE}"

echo "Postgres is starting as container '${NAME}'. Logs:" >&2
container logs -f "${NAME}"

# Tips:
# - Health check (manual): container exec -it "${NAME}" pg_isready -U user
# - Connect: PGPASSWORD=password psql -h 127.0.0.1 -U user -d land_contract -p 5432 -c '\l'