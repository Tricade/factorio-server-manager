#!/bin/bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")

make -C "$repo_root" build
docker build \
  --file "$repo_root/docker/Dockerfile-local" \
  --tag factorio-server-manager:dev \
  "$repo_root"
