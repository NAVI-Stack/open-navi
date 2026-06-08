.PHONY: test test-integration test-e2e test-e2e-live test-frontend build build-cli build-console build-all cli generate generate-python generate-ts langguard conformance clean clean-console daemon-start daemon-stop daemon-status daemon-logs docker-up docker-up-strict docker-down docker-down-strict force-reset force-reset-docker

# Native builds (local daemon / development / tests)

VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
NAVID_BIN := bin/navid
ifeq ($(OS),Windows_NT)
NAVID_BIN := bin/navid.exe
endif

# Build navid binary for local daemon mode, tests, and debugging.
build:
	go build $(LDFLAGS) -o $(NAVID_BIN) ./cmd/navid

# Build the NAVI CLI client (NaviExe).
build-cli:
	go build $(LDFLAGS) -o bin/navi.exe ./cmd/navi

# Build the NAVI Console frontend (pnpm workspace rooted at web-src/).
build-console:
	cd web-src && pnpm install --frozen-lockfile && pnpm run build

build-all: build build-cli build-console

# Run the CLI against a running NaviD (Docker or local daemon).
cli:
	go run ./cmd/navi/

# Local daemon mode developer shortcuts. User installs enter through npm or pip.
daemon-start: build build-cli
	NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon start

daemon-stop: build-cli
	NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon stop

daemon-status: build-cli
	NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon status

daemon-logs: build-cli
	NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon logs

# Testing

test:
	go test ./cmd/... ./internal/... ./connectors/... ./plugins/... -count=1

test-integration:
	go test -tags=integration ./test/integration/... -count=1

test-e2e:
	go test -tags=e2e ./test/e2e/scenarios/... -count=1 -v

# Frontend tests: typecheck + vitest (incl. accessibility) for the @navi/ui
# library and the Console. Gated in CI by the `frontend` job.
test-frontend:
	cd web-src && pnpm install --frozen-lockfile
	cd web-src && pnpm --filter @navi/ui run typecheck && pnpm --filter @navi/ui test
	cd web-src && pnpm --filter navi-console exec tsc --noEmit && pnpm --filter navi-console test

test-e2e-live:
	NAVI_E2E_LIVE=1 go test -tags='e2e e2e_live' ./test/e2e/live/... -count=1 -v

# Docker (containerized NaviD runtime)

docker-up:
	docker compose -f compose.yml up --build -d

docker-up-strict:
	docker compose -f compose.strict.yml up --build -d

docker-down:
	docker compose -f compose.yml down

docker-down-strict:
	docker compose -f compose.strict.yml down

# Factory reset via scripts/reset-navi.sh (respects .env NAVI_DATA_DRIVE / NAVI_DATA_DIR).
force-reset:
	./scripts/reset-navi.sh
	@echo "Start local mode with: make daemon-start"

force-reset-docker:
	./scripts/reset-navi.sh --start-docker

# Codegen

# Regenerate all governed contracts (Python + TypeScript) from the canonical Go
# types. Both emitters share one input set in schema/codegen.
generate: generate-python generate-ts

generate-python:
	cd schema/python && pip install -r requirements.txt && python generate.py

# Governed TypeScript types + Zod for the browser Console, emitted from the same
# Go source as generate-python (schema/codegen). Pure Go — no node tooling needed.
generate-ts:
	go run ./schema/ts/gen -root . -out web-src/navi-console/src/types/generated/governed.ts

# Conformance — language-layer contract guards (docs/architecture/language-layer-contract.md §8)

# Build the language-layer conformance checker to bin/.
langguard:
	go build $(LDFLAGS) -o bin/langguard ./cmd/langguard

# Build then run the guard over the repo. Fails on any contract violation:
# forbidden Python imports, ungoverned reads, hand-edited generated contracts.
conformance: langguard
	./bin/langguard -root .

# Cleanup

clean: clean-console
	rm -rf bin/

clean-console:
	rm -rf web/index.html web/assets
