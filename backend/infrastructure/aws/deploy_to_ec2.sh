#!/usr/bin/env bash
set -euo pipefail

DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:?Missing DOCKERHUB_USERNAME}"
GIT_SHA="${GIT_SHA:?Missing GIT_SHA}"
DEPLOY_DIR="${DEPLOY_DIR:?Missing DEPLOY_DIR}"

COMPOSE_FILE="docker-compose.yml"
DEPLOY_OVERRIDE="docker-compose.deploy.yml"
COMPOSE_CMD=(docker compose -f "${COMPOSE_FILE}")
if [ -f "${DEPLOY_OVERRIDE}" ]; then
  COMPOSE_CMD+=(-f "${DEPLOY_OVERRIDE}")
fi

echo "🚀 Deploying to EC2..."
echo "User: ${DOCKERHUB_USERNAME}"
echo "Tag:  ${GIT_SHA}"
echo "Dir:  ${DEPLOY_DIR}"

cd "${DEPLOY_DIR}"

# -----------------------------
# Ensure Docker is installed
# -----------------------------
if ! command -v docker &> /dev/null; then
  echo "[deploy] Installing Docker..."
  sudo dnf install -y docker
  sudo systemctl enable docker
  sudo systemctl start docker
  sudo usermod -aG docker ec2-user
fi

# -----------------------------
# Ensure Docker Compose v2
# -----------------------------
if ! docker compose version &> /dev/null; then
  echo "[deploy] Installing Docker Compose plugin..."
  sudo dnf install -y docker-compose-plugin || true
fi

# -----------------------------
# Login to DockerHub (optional if private)
# -----------------------------
echo "[deploy] Logging into DockerHub..."
echo "${DOCKERHUB_TOKEN:-}" | docker login -u "${DOCKERHUB_USERNAME}" --password-stdin || true

# -----------------------------
# Pull & Deploy (sha-pinned)
# -----------------------------
if [ ! -f "${DEPLOY_OVERRIDE}" ]; then
  echo "❌ Error: ${DEPLOY_OVERRIDE} not found — refusing to deploy unpinned images"
  echo "   Copy backend/docker-compose.deploy.yml to ${DEPLOY_DIR} first."
  exit 1
fi

if [ -z "${GIT_SHA:-}" ]; then
  echo "❌ Error: GIT_SHA is required — refusing to deploy unpinned images"
  exit 1
fi

echo "[deploy] Pulling images at tag ${GIT_SHA}..."
"${COMPOSE_CMD[@]}" pull

echo "[deploy] Starting containers..."
"${COMPOSE_CMD[@]}" up -d --remove-orphans

echo "[deploy] Containers status:"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'

echo "✅ Deployment complete (tag ${GIT_SHA})"
