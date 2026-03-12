#!/usr/bin/env bash
set -euo pipefail

# 创建数据目录（用于持久化）
mkdir -p "$(pwd)/etcd-data"

container run -d \
  --name etcd \
  --user 0:0 \
  --publish 2379:2379 \
  --publish 2380:2380 \
  --mount type=bind,src="$(pwd)/etcd-data",dst=/bitnami/etcd \
  --env ALLOW_NONE_AUTHENTICATION=no \
  --env ETCD_ROOT_PASSWORD=123456 \
  --env ETCD_NAME=etcd-node \
  --env ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379 \
  --env ETCD_LISTEN_PEER_URLS=http://0.0.0.0:2380 \
  bitnami/etcd:latest