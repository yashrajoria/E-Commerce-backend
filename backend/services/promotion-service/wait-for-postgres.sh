#!/bin/bash
set -e

host="${POSTGRES_HOST:-postgres}"
port="${POSTGRES_PORT:-5432}"
max_attempts=30
attempt_num=1

echo "Waiting for Postgres at $host:$port..."

until nc -z "$host" "$port"; do
  echo "Postgres is unavailable - sleeping 2 seconds..."
  sleep 2
  attempt_num=$((attempt_num+1))
  if [ $attempt_num -gt $max_attempts ]; then
    echo "Postgres still unavailable after $max_attempts attempts, exiting."
    exit 1
  fi
done

echo "Postgres is up - continuing..."
