# #!/usr/bin/env bash
# # Deploy helper for EC2 (opinionated template)
# # This script assumes the host runs Docker and optionally Docker Compose.
# # Review and adapt before using in production.

# set -euo pipefail

# # Configuration (can be provided via environment variables)
# DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:-yash263}"
# GIT_SHA="${GIT_SHA:-latest}"
# SERVICES=(auth-service bff-service cart-service inventory-service order-service payment-service product-service user-service)
# COMPOSE_FILE="docker-compose.prod.yml"
# # Ensure Docker and Docker Compose are installed
# SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# if ! command -v docker &> /dev/null || ! command -v docker-compose &> /dev/null; then
#   echo "[deploy] Installing Docker and Docker Compose..."
#   if grep -qi 'amazon linux' /etc/os-release; then
#     bash "$SCRIPT_DIR/install-docker-amazon-linux.sh"
#   else
#     bash "$SCRIPT_DIR/install-docker.sh"
#   fi
#    # Ensure docker compose v2 plugin is available
#    if ! docker compose version &> /dev/null; then
#      echo "[deploy] Installing Docker Compose v2 plugin..."
#      DOCKER_COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep 'tag_name' | cut -d '"' -f4)
#      sudo curl -L "https://github.com/docker/compose/releases/download/${DOCKER_COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/libexec/docker/cli-plugins/docker-compose
#      sudo chmod +x /usr/local/libexec/docker/cli-plugins/docker-compose
#      docker compose version
#    fi
# fi

# echo "[deploy] pulling images from Docker Hub"
# for svc in "${SERVICES[@]}"; do
#   IMG="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
#   LATEST_IMG="${DOCKERHUB_USERNAME}/${svc}:latest"
#   echo "[deploy] pulling ${IMG} and ${LATEST_IMG}"
#   docker pull "${IMG}" || true
#   docker pull "${LATEST_IMG}" || true
# done

# if [ -f "${COMPOSE_FILE}" ]; then
#   echo "[deploy] found ${COMPOSE_FILE} — recreating services with docker compose"
#   # Make sure docker compose v2 plugin is available (docker compose)
#   docker compose -f "${COMPOSE_FILE}" pull || true
#   docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans --finite-timeout || docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans || true
# else
#   echo "[deploy] ${COMPOSE_FILE} not found — attempting to restart containers individually"
#   for svc in "${SERVICES[@]}"; do
#     CONTAINER_NAME="${svc}"
#     IMG="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
#     echo "[deploy] restarting ${CONTAINER_NAME} from ${IMG}"
#     if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
#       docker stop "${CONTAINER_NAME}" || true
#       docker rm "${CONTAINER_NAME}" || true
#     fi
#     # Example run command — replace ports/env/volumes with real values for your host
#     docker run -d --env-file /home/ec2-user/E-Commerce-backend/backend/.env --name "${CONTAINER_NAME}" "${IMG}" || docker run -d --env-file /home/ec2-user/E-Commerce-backend/backend/.env --name "${CONTAINER_NAME}" "${DOCKERHUB_USERNAME}/${svc}:latest"
#   # Redis installation instructions (run once on EC2):
#   # sudo yum update -y
#   # sudo yum install -y redis
#   # sudo systemctl start redis
#   # sudo systemctl enable redis
#   done
# fi

# echo "[deploy] migrations should be executed here if needed"
# echo "[deploy] Example: run a migration container or ssh to a migration runner that has DB network access"

# echo "[deploy] post-deploy health checks (localhost:8080 -> api-gateway)"
# if curl -fS --max-time 5 http://localhost:8080/health >/dev/null 2>&1; then
#   echo "[ok] api-gateway /health OK"
# else
#   echo "[warn] api-gateway /health failed — inspect containers and logs"
#   docker ps --format 'table {{.Names}}	{{.Image}}	{{.Status}}' || true
# fi

# echo "[deploy] finished"


#!/usr/bin/env bash
# Deploy helper for EC2
# This script deploys ShopSwift backend services using Docker Compose
# Assumes the script is run from the deployment directory containing docker-compose.yml

set -euo pipefail

# Configuration (provided via environment variables from GitHub Actions)
DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:-yash263}"
GIT_SHA="${GIT_SHA:-latest}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/${USER}/E-Commerce-backend/backend}"
SERVICES=(auth-service bff-service cart-service inventory-service order-service payment-service product-service user-service)
COMPOSE_FILE="docker-compose.yml"

# Script directory for helper scripts
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "============================================"
echo "ShopSwift Backend Deployment"
echo "============================================"
echo "DOCKERHUB_USERNAME: ${DOCKERHUB_USERNAME}"
echo "GIT_SHA: ${GIT_SHA}"
echo "DEPLOY_DIR: ${DEPLOY_DIR}"
echo "COMPOSE_FILE: ${COMPOSE_FILE}"
echo "============================================"

# Ensure we're in the correct directory
cd "${DEPLOY_DIR}"

# ============================================
# 1. Install Docker and Docker Compose if needed
# ============================================
if ! command -v docker &> /dev/null; then
  echo "[deploy] Docker not found - installing..."
  
  if grep -qi 'amazon linux' /etc/os-release 2>/dev/null; then
    echo "[deploy] Detected Amazon Linux"
    if [ -f "${SCRIPT_DIR}/install-docker-amazon-linux.sh" ]; then
      bash "${SCRIPT_DIR}/install-docker-amazon-linux.sh"
    else
      # Fallback inline installation for Amazon Linux
      sudo yum update -y
      sudo yum install -y docker
      sudo systemctl start docker
      sudo systemctl enable docker
      sudo usermod -aG docker ${USER}
    fi
  else
    echo "[deploy] Detected Ubuntu/Debian"
    if [ -f "${SCRIPT_DIR}/install-docker.sh" ]; then
      bash "${SCRIPT_DIR}/install-docker.sh"
    else
      # Fallback inline installation for Ubuntu/Debian
      sudo apt-get update
      sudo apt-get install -y ca-certificates curl gnupg
      sudo install -m 0755 -d /etc/apt/keyrings
      curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
      sudo chmod a+r /etc/apt/keyrings/docker.gpg
      
      echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
        $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
        sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
      
      sudo apt-get update
      sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      sudo usermod -aG docker ${USER}
    fi
  fi
  
  echo "[deploy] Docker installed successfully"
fi

# Ensure Docker Compose v2 plugin is available
if ! docker compose version &> /dev/null; then
  echo "[deploy] Installing Docker Compose v2 plugin..."
  
  DOCKER_COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep 'tag_name' | cut -d '"' -f4)
  sudo mkdir -p /usr/local/lib/docker/cli-plugins
  sudo curl -SL "https://github.com/docker/compose/releases/download/${DOCKER_COMPOSE_VERSION}/docker-compose-linux-$(uname -m)" \
    -o /usr/local/lib/docker/cli-plugins/docker-compose
  sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  
  echo "[deploy] Docker Compose installed: $(docker compose version)"
fi

# ============================================
# 2. Pull Docker images from Docker Hub
# ============================================
echo "[deploy] Pulling Docker images from Docker Hub..."

PULL_ERRORS=0

for svc in "${SERVICES[@]}"; do
  IMG_SHA="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
  IMG_LATEST="${DOCKERHUB_USERNAME}/${svc}:latest"
  
  echo "[deploy] Pulling ${IMG_SHA}..."
  if docker pull "${IMG_SHA}"; then
    echo "[deploy] ✅ Successfully pulled ${IMG_SHA}"
  else
    echo "[deploy] ⚠️  Failed to pull ${IMG_SHA}, trying latest..."
    if docker pull "${IMG_LATEST}"; then
      echo "[deploy] ✅ Successfully pulled ${IMG_LATEST}"
    else
      echo "[deploy] ❌ Failed to pull both ${IMG_SHA} and ${IMG_LATEST}"
      PULL_ERRORS=$((PULL_ERRORS + 1))
    fi
  fi
done

if [ $PULL_ERRORS -gt 0 ]; then
  echo "[deploy] ⚠️  Warning: Failed to pull $PULL_ERRORS image(s)"
fi

# ============================================
# 3. Deploy using Docker Compose or individual containers
# ============================================
if [ -f "${COMPOSE_FILE}" ]; then
  echo "[deploy] Found ${COMPOSE_FILE} - deploying with Docker Compose..."
  
  # Set the GIT_SHA as an environment variable for docker-compose
  export IMAGE_TAG="${GIT_SHA}"
  
  # Pull images defined in compose file
  docker compose -f "${COMPOSE_FILE}" pull || echo "[deploy] ⚠️  Some images may not have pulled"
  
  # Deploy services
  echo "[deploy] Starting services with Docker Compose..."
  if docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans; then
    echo "[deploy] ✅ Services started successfully"
  else
    echo "[deploy] ❌ Failed to start services with Docker Compose"
    docker ps -a
    exit 1
  fi
  
  # Clean up old images and containers
  echo "[deploy] Cleaning up old images..."
  docker image prune -f || true
  
else
  echo "[deploy] ⚠️  ${COMPOSE_FILE} not found - deploying containers individually..."
  
  ENV_FILE="${DEPLOY_DIR}/.env"
  
  if [ ! -f "${ENV_FILE}" ]; then
    echo "[deploy] ❌ Error: .env file not found at ${ENV_FILE}"
    exit 1
  fi
  
  for svc in "${SERVICES[@]}"; do
    CONTAINER_NAME="${svc}"
    IMG_SHA="${DOCKERHUB_USERNAME}/${svc}:${GIT_SHA}"
    IMG_LATEST="${DOCKERHUB_USERNAME}/${svc}:latest"
    
    echo "[deploy] Deploying ${CONTAINER_NAME}..."
    
    # Stop and remove existing container
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
      echo "[deploy] Stopping existing ${CONTAINER_NAME}..."
      docker stop "${CONTAINER_NAME}" || true
      docker rm "${CONTAINER_NAME}" || true
    fi
    
    # Try to run with SHA tag first, fallback to latest
    echo "[deploy] Starting ${CONTAINER_NAME} with image ${IMG_SHA}..."
    if ! docker run -d --env-file "${ENV_FILE}" --name "${CONTAINER_NAME}" "${IMG_SHA}"; then
      echo "[deploy] ⚠️  Failed with SHA tag, trying latest..."
      docker run -d --env-file "${ENV_FILE}" --name "${CONTAINER_NAME}" "${IMG_LATEST}"
    fi
  done
  
  echo "[deploy] ✅ All containers deployed individually"
fi

# ============================================
# 4. Wait for services to be ready
# ============================================
echo "[deploy] Waiting for services to start..."
sleep 5

# ============================================
# 5. Display deployment status
# ============================================
echo ""
echo "============================================"
echo "Deployment Status"
echo "============================================"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
echo "============================================"

# ============================================
# 6. Optional: Run database migrations
# ============================================
echo "[deploy] Database migrations placeholder"
echo "[deploy] TODO: Execute migrations if needed"
# Example:
# docker run --rm --env-file "${DEPLOY_DIR}/.env" \
#   "${DOCKERHUB_USERNAME}/migration-runner:${GIT_SHA}" \
#   migrate -path /migrations -database "${DATABASE_URL}" up

# ============================================
# 7. Quick health check
# ============================================
echo "[deploy] Running quick health check..."
sleep 3

if curl -fS --max-time 5 http://localhost:8080/health >/dev/null 2>&1; then
  echo "[deploy] ✅ api-gateway /health OK"
else
  echo "[deploy] ⚠️  api-gateway /health failed - check logs"
  echo "[deploy] Recent api-gateway logs:"
  docker logs --tail 20 api-gateway 2>&1 || echo "Could not fetch logs"
fi

echo ""
echo "============================================"
echo "✅ Deployment Complete"
echo "============================================"
echo "To view logs: docker logs -f <service-name>"
echo "To check status: docker ps"
echo "To restart a service: docker compose restart <service-name>"
echo "============================================"
