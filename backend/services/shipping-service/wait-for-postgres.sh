#!/bin/sh
set -e

HOST="${POSTGRES_HOST:-postgres}"
PORT="${POSTGRES_PORT:-5432}"

echo "Waiting for PostgreSQL at $HOST:$PORT..."
while ! nc -z "$HOST" "$PORT"; do
  sleep 1
done
echo "PostgreSQL is ready!"

exec "$@"
