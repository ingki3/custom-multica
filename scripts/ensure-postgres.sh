#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing env file: $ENV_FILE"
  echo "Create .env from .env.example, or run 'make worktree-env' and use .env.worktree."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

POSTGRES_DB="${POSTGRES_DB:-multica}"
POSTGRES_USER="${POSTGRES_USER:-multica}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-multica}"
DATABASE_URL="${DATABASE_URL:-}"

export PGPASSWORD="$POSTGRES_PASSWORD"

db_host=""
db_port="${POSTGRES_PORT:-5432}"
db_name="$POSTGRES_DB"

parse_database_url() {
  local rest authority hostport path port_part

  rest="${DATABASE_URL#*://}"
  rest="${rest%%\?*}"
  authority="${rest%%/*}"
  path="${rest#*/}"

  if [ "$authority" = "$rest" ]; then
    path=""
  fi

  hostport="${authority##*@}"

  if [[ "$hostport" == \[* ]]; then
    db_host="${hostport#\[}"
    db_host="${db_host%%]*}"
    port_part="${hostport#*\]}"
    if [[ "$port_part" == :* ]] && [ -n "${port_part#:}" ]; then
      db_port="${port_part#:}"
    fi
  else
    db_host="${hostport%%:*}"
    if [[ "$hostport" == *:* ]] && [ -n "${hostport##*:}" ]; then
      db_port="${hostport##*:}"
    fi
  fi

  if [ -n "$path" ]; then
    db_name="${path%%/*}"
  fi
}

if [ -n "$DATABASE_URL" ]; then
  parse_database_url
fi

is_local() {
  [ -z "$DATABASE_URL" ] || [ "$db_host" = "localhost" ] || [ "$db_host" = "127.0.0.1" ] || [ "$db_host" = "::1" ]
}

# Check if a native (non-Docker) PostgreSQL is reachable on the target port.
native_pg_available() {
  if command -v pg_isready > /dev/null 2>&1; then
    pg_isready -h localhost -p "$db_port" -U "$POSTGRES_USER" > /dev/null 2>&1
  else
    return 1
  fi
}

# Create the target database via psql if it doesn't exist.
ensure_db_via_psql() {
  local psql_cmd="psql -h localhost -p $db_port -U $POSTGRES_USER -d postgres"

  echo "==> Ensuring database '$POSTGRES_DB' exists..."
  db_exists="$($psql_cmd -Atqc "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DB'" 2>/dev/null || true)"

  if [ "$db_exists" != "1" ]; then
    $psql_cmd -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$POSTGRES_DB\"" > /dev/null
  fi
}

# Detect $COMPOSE command variant
if $COMPOSE version >/dev/null 2>&1; then
  COMPOSE="$COMPOSE"
else
  COMPOSE="docker-compose"
fi

if is_local; then
  # ---------- Local: try native PostgreSQL first, fall back to Docker ----------
  if native_pg_available; then
    echo "==> Native PostgreSQL detected on localhost:$db_port"
    ensure_db_via_psql
    echo "✓ PostgreSQL ready (native). Database: $POSTGRES_DB"
  elif command -v docker > /dev/null 2>&1; then
    echo "==> No native PostgreSQL on localhost:$db_port. Using Docker..."
    echo "==> Ensuring shared PostgreSQL container is running on localhost:$db_port..."
    $COMPOSE up -d postgres

    echo "==> Waiting for PostgreSQL to be ready..."
    until $COMPOSE exec -T postgres pg_isready -U "$POSTGRES_USER" -d postgres > /dev/null 2>&1; do
      sleep 1
    done

    echo "==> Ensuring database '$POSTGRES_DB' exists..."
    db_exists="$($COMPOSE exec -T postgres \
      psql -U "$POSTGRES_USER" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DB'")"

    if [ "$db_exists" != "1" ]; then
      $COMPOSE exec -T postgres \
        psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 \
        -c "CREATE DATABASE \"$POSTGRES_DB\"" \
        > /dev/null
    fi

    echo "✓ PostgreSQL ready (Docker). Database: $POSTGRES_DB"
  else
    echo "✗ No PostgreSQL running on localhost:$db_port and Docker is not installed."
    echo ""
    echo "Options:"
    echo "  1. Install and start PostgreSQL 17 with pgvector natively"
    echo "  2. Install Docker and let the dev tooling manage it"
    echo ""
    echo "Native PostgreSQL setup (macOS):"
    echo "  brew install postgresql@17"
    echo "  brew services start postgresql@17"
    echo "  createuser -s $POSTGRES_USER  # if needed"
    echo ""
    echo "Native PostgreSQL setup (Ubuntu/Debian):"
    echo "  sudo apt install postgresql-17 postgresql-17-pgvector"
    echo "  sudo systemctl start postgresql"
    exit 1
  fi
else
  # ---------- Remote: skip Docker, verify connectivity ----------
  echo "==> Remote database detected (host: $db_host). Skipping Docker."
  if command -v pg_isready > /dev/null 2>&1; then
    echo "==> Waiting for PostgreSQL at $db_host:$db_port to be ready..."
    until pg_isready -d "$DATABASE_URL" > /dev/null 2>&1; do
      sleep 1
    done
    echo "✓ PostgreSQL ready (remote: $db_host:$db_port). Database: $db_name"
  else
    echo "==> pg_isready not found. Skipping remote connectivity preflight."
    echo "✓ PostgreSQL configured (remote: $db_host:$db_port). Database: $db_name"
  fi
fi
