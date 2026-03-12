#!/usr/bin/env bash
set -euo pipefail

# This script mirrors third_party/redis/docker-compose/docker-compose.yml
# Runtime: Apple "container" CLI
# Service: redis:7.4.0
# Name: redis-label

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONF_DIR="${SCRIPT_DIR}/conf"
DATA_HOST="${SCRIPT_DIR}/redis-data"

mkdir -p "${DATA_HOST}"
# Relax permissions so redis user (UID 999) can write without chown inside container
chmod 0777 "${DATA_HOST}" || true

# Remove previous container if exists (ignore errors)
container rm -f redis >/dev/null 2>&1 || true

# Run Redis (as root to avoid potential permission issues on bind mounts)
container run -d \
  --name redis \
  --user 999:999 \
  --publish 6379:6379 \
  --mount type=bind,src="${DATA_HOST}",dst=/data \
  --mount type=bind,src="${CONF_DIR}",dst=/usr/local/etc/redis/redis.conf,ro=true \
  redis:7.4.0 \
  redis-server /usr/local/etc/redis/redis.conf

echo "Redis is starting as container 'redis'. Logs:" >&2
container logs -f redis
