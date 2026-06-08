#!/bin/bash

set -euo pipefail

# #########################################################
# Relaunch the NAVI daemon and CLI
# 
# This script will build the navi binaries and start the docker container.
# It will then smoke test the container and print the version of the navi binaries and docker container.
# It will then print the version of the docker container.
# #########################################################
if [ -t 1 ]; then
  clear
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAVI_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$NAVI_DIR"

# Build the navi binaries
go build -o bin/navid ./cmd/navid
go build -o bin/navi.exe ./cmd/navi

# Determine the docker command to use. Git Bash on Windows should prefer
# docker.exe, but some POSIX environments expose a non-runnable docker.exe shim.
DOCKER=""
for candidate in docker.exe docker; do
  if command -v "$candidate" >/dev/null 2>&1 && "$candidate" compose version >/dev/null 2>&1; then
    DOCKER="$candidate"
    break
  fi
done
if [ -z "$DOCKER" ]; then
  DOCKER="docker"
fi

# Ensure Docker Desktop's Linux engine is available before compose commands.
if ! "$DOCKER" version >/tmp/navi-docker-version.txt 2>/tmp/navi-docker-version.err; then
  echo "Error: Docker is not available." >&2
  if grep -q "dockerDesktopLinuxEngine" /tmp/navi-docker-version.err; then
    echo "Docker Desktop's Linux engine is not running. Start Docker Desktop, wait for it to finish booting, and make sure it is using Linux containers." >&2
  else
    cat /tmp/navi-docker-version.err >&2
  fi
  rm -f /tmp/navi-docker-version.txt /tmp/navi-docker-version.err
  exit 1
fi
rm -f /tmp/navi-docker-version.txt /tmp/navi-docker-version.err

# Initialize ~/.navi directory with defaults if empty
mkdir -p ~/.navi/data ~/.navi/config ~/.navi/skills
if [ -d "config" ] && [ ! -f ~/.navi/config/config.yaml ] && [ ! -f ~/.navi/config/runtime.yaml ]; then
  cp -R config/* ~/.navi/config/ || true
fi
if [ -d "skills" ] && [ -z "$(ls -A ~/.navi/skills 2>/dev/null)" ]; then
  cp -R skills/* ~/.navi/skills/ || true
fi

# Relaunch through Compose so Docker can tear down its own port-forwarding cleanly.
"$DOCKER" compose down --remove-orphans >/dev/null 2>&1 || true
"$DOCKER" compose up --build -d

# Wait for container and app to start up. Give it up to 10 seconds.
for i in {1..10}; do
  status_code="$(curl -s -o /dev/null -w "%{http_code}" http://localhost:6284/health || true)"
  if [[ "$status_code" == "200" ]]; then
    break
  fi
  sleep 1
done
status_code="$(curl -s -o /dev/null -w "%{http_code}" http://localhost:6284/health || true)"
if [[ "$status_code" != "200" ]]; then
  echo "Error: navid did not become healthy on http://localhost:6284/health (status: $status_code)." >&2
  exit 1
fi
echo "navid is healthy on http://localhost:6284/health"


# #########################################################
# End of script
# #########################################################
