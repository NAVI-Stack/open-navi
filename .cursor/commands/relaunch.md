# Rebuild CLI and restart NaviD (Docker)

Rebuild the NaviExe CLI and the NaviD Docker image; restart the container. Run from **this repo root** (`projects/navi/`).

```bash
make build-cli && docker compose -f compose.yml up --build -d
```

Without `make` (e.g. Windows):

```bash
go build -o bin/navi.exe ./cmd/navi
docker compose -f compose.yml up --build -d
```

- **build-cli** / `go build ... ./cmd/navi`: builds `bin/navi.exe` (NaviExe client only).
- **docker compose up --build -d**: rebuilds the NaviD image and starts the container; the supported runtime is Docker Compose, not a host `navid` binary.

Smoke test: `curl -s -o /dev/null -w "%{http_code}" http://localhost:6284/health` (expect `200`).
