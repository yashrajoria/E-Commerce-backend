#!/bin/bash
# install-docker-amazon-linux.sh
# Installs Docker and Docker Compose on Amazon Linux EC2
set -e

# Update package info
sudo yum update -y

# Install Docker
sudo amazon-linux-extras install docker -y
sudo yum install -y docker

# Enable and start Docker
sudo systemctl enable docker
sudo systemctl start docker

# Install Docker Compose (latest)
sudo yum install -y curl
DOCKER_COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep 'tag_name' | cut -d '"' -f4)
sudo curl -L "https://github.com/docker/compose/releases/download/${DOCKER_COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

docker --version
docker-compose --version

echo "Docker and Docker Compose installed successfully."
