#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$REPO_ROOT"

# ---------- Parse flags ----------
SKIP_BUILD=false
for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=true ;;
    -h|--help)
      echo "Usage: ./prod.sh [--skip-build]"
      echo ""
      echo "Build and start backend + frontend in production mode."
      echo "Auto-detects .env.worktree (worktree) or .env (main checkout)."
      echo ""
      echo "Options:"
      echo "  --skip-build  Skip build steps, start from existing artifacts"
      echo "  -h, --help    Show this help"
      exit 0
      ;;
  esac
done

# ---------- Check prerequisites ----------
missing=()
command -v node >/dev/null 2>&1 || missing+=("node")
command -v pnpm >/dev/null 2>&1 || missing+=("pnpm")
command -v go >/dev/null 2>&1 || missing+=("go")

if [ ${#missing[@]} -gt 0 ]; then
  echo "✗ Missing prerequisites: ${missing[*]}"
  exit 1
fi

# ---------- Environment file ----------
if [ -f .git ]; then
  ENV_FILE=".env.worktree"
else
  ENV_FILE=".env"
fi

if [ ! -f "$ENV_FILE" ]; then
  echo "✗ Missing env file: $ENV_FILE"
  exit 1
fi

echo "==> Using $ENV_FILE"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

PORT="${PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

# ---------- Database ----------
bash scripts/ensure-postgres.sh "$ENV_FILE"

# ---------- Build ----------
if [ "$SKIP_BUILD" = false ]; then
  echo "==> Building Go backend..."
  cd server
  go build -o bin/server ./cmd/server
  go build -o bin/migrate ./cmd/migrate
  go build -o bin/multica ./cmd/multica
  cd "$REPO_ROOT"

  echo "==> Building Next.js (standalone)..."
  export STANDALONE=true
  pnpm --filter @multica/web build
fi

# ---------- Verify artifacts ----------
if [ ! -f server/bin/server ]; then
  echo "✗ server/bin/server not found. Run without --skip-build first."
  exit 1
fi
if [ ! -f apps/web/.next/standalone/apps/web/server.js ]; then
  echo "✗ apps/web/.next/standalone/apps/web/server.js not found. Run without --skip-build first."
  exit 1
fi

# ---------- Migrate ----------
echo "==> Running migrations..."
./server/bin/migrate up

# ---------- Start ----------
echo "==> Starting production services..."
echo "    Backend:  http://localhost:$PORT"
echo "    Frontend: http://localhost:$FRONTEND_PORT"
echo ""

PIDS=()

cleanup() {
  local status="${1:-$?}"
  trap - INT TERM EXIT

  if [ "${#PIDS[@]}" -gt 0 ]; then
    echo ""
    echo "==> Stopping production services..."

    for pid in "${PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
      fi
    done

    for _ in 1 2 3 4 5; do
      local running=false
      for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
          running=true
          break
        fi
      done
      if [ "$running" = false ]; then
        exit "$status"
      fi
      sleep 1
    done

    for pid in "${PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    done
  fi

  exit "$status"
}

trap 'cleanup 130' INT
trap 'cleanup 143' TERM
trap 'cleanup $?' EXIT

./server/bin/server &
PIDS+=("$!")

# Next.js standalone needs static assets alongside it
if [ ! -d apps/web/.next/standalone/apps/web/.next/static ]; then
  mkdir -p apps/web/.next/standalone/apps/web/.next
  cp -r apps/web/.next/static apps/web/.next/standalone/apps/web/.next/static
fi
if [ ! -d apps/web/.next/standalone/apps/web/public ] && [ -d apps/web/public ]; then
  mkdir -p apps/web/.next/standalone/apps/web
  cp -r apps/web/public apps/web/.next/standalone/apps/web/public
fi

PORT="$FRONTEND_PORT" HOSTNAME=0.0.0.0 node apps/web/.next/standalone/apps/web/server.js &
PIDS+=("$!")

# Start local daemon
echo "==> Starting daemon..."
./server/bin/multica daemon start --foreground &
PIDS+=("$!")

wait
