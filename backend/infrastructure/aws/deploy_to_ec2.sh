#!/usr/bin/env bash
# Deploy helper for EC2 (opinionated template)
# This script assumes the host runs Docker and optionally Docker Compose.
# Review and adapt before using in production.

set -euo pipefail

# Configuration (can be provided via environment variables)
DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:-your-dockerhub-username}"
GIT_SHA="${GIT_SHA:-latest}"
SERVICES=(auth-service bff-service cart-service inventory-service order-service payment-service product-service user-service)
COMPOSE_FILE="docker-compose.prod.yml"

echo "[deploy] pulling images from Docker Hub"
for svc in "${SERVICES[@]}"; do
  IMG="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
  LATEST_IMG="${DOCKERHUB_USERNAME}/${svc}:latest"
  echo "[deploy] pulling ${IMG} and ${LATEST_IMG}"
  docker pull "${IMG}" || true
  docker pull "${LATEST_IMG}" || true
done

if [ -f "${COMPOSE_FILE}" ]; then
  echo "[deploy] found ${COMPOSE_FILE} — recreating services with docker compose"
  # Make sure docker compose v2 plugin is available (docker compose)
  docker compose -f "${COMPOSE_FILE}" pull || true
  docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans --finite-timeout || docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans || true
else
  echo "[deploy] ${COMPOSE_FILE} not found — attempting to restart containers individually"
  for svc in "${SERVICES[@]}"; do
    CONTAINER_NAME="${svc}"
    IMG="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
    echo "[deploy] restarting ${CONTAINER_NAME} from ${IMG}"
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
      docker stop "${CONTAINER_NAME}" || true
      docker rm "${CONTAINER_NAME}" || true
    fi
    # Example run command — replace ports/env/volumes with real values for your host
    docker run -d --name "${CONTAINER_NAME}" "${IMG}" || docker run -d --name "${CONTAINER_NAME}" "${DOCKERHUB_USERNAME}/${svc}:latest"
  done
fi

echo "[deploy] migrations should be executed here if needed"
echo "[deploy] Example: run a migration container or ssh to a migration runner that has DB network access"

echo "[deploy] post-deploy health checks (localhost:8080 -> api-gateway)"
if curl -fS --max-time 5 http://localhost:8080/health >/dev/null 2>&1; then
  echo "[ok] api-gateway /health OK"
else
  echo "[warn] api-gateway /health failed — inspect containers and logs"
  docker ps --format 'table {{.Names}}	{{.Image}}	{{.Status}}' || true
fi

echo "[deploy] finished"
