#!/usr/bin/env bash
# Factory reset: erase all NAVI state and data (local + Docker), then optionally start navid.
# Run from repo root.
#
# After erase + start, the instance has no authentication; the user can run onboarding from scratch.
#
# Usage:
#   ./scripts/reset-navi.sh                    # Factory reset: wipe local + Docker (stops navid, erases DB, checkpoint, JetStream, workspace)
#   ./scripts/reset-navi.sh --start            # Start navid locally (go run)
#   ./scripts/reset-navi.sh --start-docker     # Start navid via Docker (can also use --start --docker)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAVI_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

USE_DOCKER=false
DO_START=false
for arg in "$@"; do
  case "$arg" in
    --docker) USE_DOCKER=true ;;
    --start)  DO_START=true ;;
    --start-docker) DO_START=true; USE_DOCKER=true ;;
  esac
done

if [[ ! -d "$NAVI_DIR" ]]; then
  echo "Error: repo root not found at $NAVI_DIR" >&2
  exit 1
fi

cd "$NAVI_DIR"

# Load compose .env so NAVI_DATA_DRIVE / NAVI_DATA_DIR match docker compose bind mounts.
load_dotenv() {
  if [[ -f "$NAVI_DIR/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$NAVI_DIR/.env"
    set +a
  fi
}

# Resolve host data paths using the same defaults as compose.yml.
resolve_data_paths() {
  local drive="${NAVI_DATA_DRIVE:-./.navi}"
  if [[ "$drive" != /* && "$drive" != [A-Za-z]:* ]]; then
    drive="$NAVI_DIR/$drive"
  fi
  DATA_ROOT="$drive"
  if [[ -n "${NAVI_DATA_DIR:-}" ]]; then
    DATA_DIR="$NAVI_DATA_DIR"
    if [[ "$DATA_DIR" != /* && "$DATA_DIR" != [A-Za-z]:* ]]; then
      DATA_DIR="$NAVI_DIR/$DATA_DIR"
    fi
  else
    DATA_DIR="$drive/data"
  fi
}

# Stop any local (non-Docker) process on the navid gateway port.
# Only called in local (non-Docker) mode — never in Docker mode, to avoid
# accidentally killing Docker Desktop's port-forwarding proxy processes.
kill_port_6284_local() {
  local port=6284
  if [[ "$(uname -s)" =~ MINGW|MSYS|CYGWIN ]]; then
    local pids
    pids=$(netstat -ano 2>/dev/null | awk "/:${port}[^0-9]/ { print \$NF }" | sort -u)
    for pid in $pids; do
      [[ -z "$pid" || "$pid" == "0" || ! "$pid" =~ ^[0-9]+$ ]] && continue
      local pname=""
      pname=$(tasklist.exe /FI "PID eq $pid" /NH 2>/dev/null) || true
      if echo "$pname" | grep -iqE 'docker|vpnkit'; then
        continue
      fi
      taskkill //PID "$pid" //F 2>/dev/null || true
    done
  else
    local pid
    pid=$(lsof -ti:"$port" 2>/dev/null) || true
    [[ -n "$pid" ]] && kill -9 $pid 2>/dev/null || true
  fi
}

# Check Docker daemon is reachable, with a clear error if not.
check_docker() {
  if ! docker version >/tmp/navi-docker-version.txt 2>/tmp/navi-docker-version.err; then
    echo "Error: Docker is not available." >&2
    if grep -q "dockerDesktopLinuxEngine" /tmp/navi-docker-version.err 2>/dev/null; then
      echo "Docker Desktop's Linux engine is not running. Start Docker Desktop, wait for it to finish booting, and make sure it is using Linux containers." >&2
    else
      cat /tmp/navi-docker-version.err >&2
    fi
    rm -f /tmp/navi-docker-version.txt /tmp/navi-docker-version.err
    exit 1
  fi
  rm -f /tmp/navi-docker-version.txt /tmp/navi-docker-version.err
}

wipe_local_state() {
  load_dotenv
  resolve_data_paths

  echo "Wiping local data (DB, checkpoint, JetStream, workspace)..."
  echo "  repo root: $NAVI_DIR"
  echo "  data root: $DATA_ROOT"
  echo "  data dir:  $DATA_DIR"

  rm -f "$NAVI_DIR/navi.db" "$NAVI_DIR/navi.db-wal" "$NAVI_DIR/navi.db-shm"
  rm -rf "$NAVI_DIR/jetstream" "$NAVI_DIR/.onboarding" "$NAVI_DIR/workspace" "$NAVI_DIR/.navi"

  if [[ "$DATA_ROOT" != "$NAVI_DIR/.navi" ]]; then
    rm -rf "$DATA_ROOT"
  fi
  if [[ -n "${NAVI_DATA_DIR:-}" && "$DATA_DIR" != "$DATA_ROOT/data" && "$DATA_DIR" != "$DATA_ROOT" ]]; then
    rm -rf "$DATA_DIR"
  fi
}

# Validate Docker early in Docker mode — before any destructive operations.
if [[ "$USE_DOCKER" == true ]]; then
  check_docker
fi

echo "Factory reset: erasing all NAVI state and data (local + Docker)..."
echo "Stopping navid container and wiping Docker state for this compose project..."
docker compose -f compose.yml down -v --remove-orphans 2>/dev/null || true
docker compose -f compose.strict.yml down -v --remove-orphans 2>/dev/null || true

if [[ "$USE_DOCKER" != true ]]; then
  # Only kill local processes in non-Docker mode — Docker Desktop owns port 6284 otherwise.
  echo "Stopping any remaining local process on port 6284 (navid)..."
  kill_port_6284_local
  sleep 1
fi

wipe_local_state
echo "Done. NAVI is fully reset."

if [[ "$DO_START" == true ]]; then
  if [[ "$USE_DOCKER" == true ]]; then
    echo "Starting navid via Docker..."
    # Brief pause — docker compose down -v can cause Docker Desktop's engine to
    # briefly reset; give it a moment before issuing compose up.
    sleep 2
    docker compose -f compose.yml up --build -d
    echo "navid is running in Docker."
  else
    echo "Starting navid..."
    if ! command -v go >/dev/null 2>&1; then
      echo "Error: go is not available. Install Go or use --start-docker with Docker running." >&2
      exit 1
    fi
    exec go run ./cmd/navid/
  fi
else
  echo "Start navid (e.g. ./scripts/reset-navi.sh --start-docker) for a fresh instance with no authentication; you can run onboarding from scratch."
fi
