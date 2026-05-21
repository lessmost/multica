#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
LABEL="com.multica.dev"
PLIST="$HOME/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="$REPO_ROOT/.launchd-logs"
DELAY_SECONDS=60

usage() {
  echo "Usage: ./launchd.sh <command>"
  echo ""
  echo "Manage the Multica production LaunchAgent (auto-start on login)."
  echo ""
  echo "Commands:"
  echo "  install     Install and load the LaunchAgent"
  echo "  start       Load the installed LaunchAgent"
  echo "  stop        Unload the LaunchAgent without removing it"
  echo "  uninstall   Unload and remove the LaunchAgent"
  echo "  status      Show LaunchAgent status"
  echo "  logs        Tail the LaunchAgent logs"
  echo "  restart     Unload + reload the LaunchAgent"
}

is_loaded() {
  launchctl print "gui/$(id -u)/${LABEL}" >/dev/null 2>&1
}

load_env() {
  if [ -f "$REPO_ROOT/.git" ]; then
    ENV_FILE="$REPO_ROOT/.env.worktree"
  else
    ENV_FILE="$REPO_ROOT/.env"
  fi

  if [ -f "$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
  fi

  PORT="${PORT:-8080}"
  FRONTEND_PORT="${FRONTEND_PORT:-3000}"
}

kill_pid() {
  local pid="$1"
  local label="$2"

  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi

  kill "$pid" 2>/dev/null || true
  for _ in 1 2 3 4 5; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "  stopped $label (pid $pid)"
      return 0
    fi
    sleep 1
  done

  kill -9 "$pid" 2>/dev/null || true
  echo "  killed $label (pid $pid)"
}

stop_matching_processes() {
  local pattern="$1"
  local label="$2"
  local pids

  pids="$(pgrep -f "$pattern" 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    return 0
  fi

  for pid in $pids; do
    if [ "$pid" != "$$" ]; then
      kill_pid "$pid" "$label"
    fi
  done
}

stop_port_processes() {
  local port="$1"
  local label="$2"
  local pids

  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    return 0
  fi

  for pid in $pids; do
    kill_pid "$pid" "$label port $port"
  done
}

stop_services() {
  load_env
  stop_matching_processes "prod.sh" "prod launcher"
  stop_matching_processes "${REPO_ROOT}/server/bin/server" "backend"
  stop_matching_processes "${REPO_ROOT}/apps/web/.next/standalone/apps/web/server.js" "frontend"
  stop_matching_processes "${REPO_ROOT}/server/bin/multica daemon start --foreground" "daemon"
  stop_port_processes "$PORT" "backend"
  stop_port_processes "$FRONTEND_PORT" "frontend"
}

do_install() {
  mkdir -p "$LOG_DIR"
  mkdir -p "$(dirname "$PLIST")"

  cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>-c</string>
    <string>sleep ${DELAY_SECONDS} &amp;&amp; exec ${REPO_ROOT}/prod.sh --skip-build</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${REPO_ROOT}</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <false/>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/stdout.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/stderr.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>HOME</key>
    <string>${HOME}</string>
  </dict>
</dict>
</plist>
EOF

  launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$PLIST"

  echo "✓ LaunchAgent installed and loaded."
  echo "  Plist:  $PLIST"
  echo "  Delay:  ${DELAY_SECONDS}s after login"
  echo "  Logs:   $LOG_DIR/"
  echo ""
  echo "  Note: uses --skip-build. Run ./prod.sh once manually first to build."
}

do_start() {
  if [ ! -f "$PLIST" ]; then
    echo "LaunchAgent not installed. Run './launchd.sh install' first."
    exit 1
  fi

  if is_loaded; then
    echo "LaunchAgent already loaded."
    exit 0
  fi

  launchctl bootstrap "gui/$(id -u)" "$PLIST"
  echo "✓ LaunchAgent loaded."
}

do_stop() {
  if [ -f "$PLIST" ] && is_loaded; then
    launchctl bootout "gui/$(id -u)" "$PLIST"
    echo "✓ LaunchAgent unloaded."
  else
    echo "LaunchAgent not loaded."
  fi

  stop_services
  echo "✓ Services stopped."
}

do_uninstall() {
  if [ ! -f "$PLIST" ]; then
    echo "LaunchAgent not installed."
    exit 0
  fi

  launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
  stop_services
  rm -f "$PLIST"
  echo "✓ LaunchAgent unloaded, services stopped, and removed."
}

do_status() {
  if [ ! -f "$PLIST" ]; then
    echo "LaunchAgent not installed."
    exit 0
  fi

  launchctl print "gui/$(id -u)/${LABEL}" 2>&1 | grep -E "state|pid|last exit" || \
    echo "Not currently loaded."
}

do_restart() {
  if [ ! -f "$PLIST" ]; then
    echo "LaunchAgent not installed. Run './launchd.sh install' first."
    exit 1
  fi

  launchctl bootout "gui/$(id -u)" "$PLIST" 2>/dev/null || true
  stop_services
  launchctl bootstrap "gui/$(id -u)" "$PLIST"
  echo "✓ LaunchAgent restarted."
}

do_logs() {
  if [ ! -d "$LOG_DIR" ]; then
    echo "No logs yet."
    exit 0
  fi
  tail -f "$LOG_DIR/stdout.log" "$LOG_DIR/stderr.log"
}

case "${1:-}" in
  install)   do_install ;;
  start)     do_start ;;
  stop)      do_stop ;;
  uninstall) do_uninstall ;;
  status)    do_status ;;
  restart)   do_restart ;;
  logs)      do_logs ;;
  *)         usage ;;
esac
