# Stage 1: frontend build
FROM node:20-alpine AS console-builder
WORKDIR /src

# Copy committed static assets (onboarding.html, etc)
COPY web/ ./web/

# Enable pnpm via corepack (version pinned by web-src/package.json "packageManager").
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable && corepack prepare pnpm@10.33.0 --activate

# Install the frontend workspace from the committed lockfile. Copy the workspace
# manifests first so the install layer is cached independently of source.
COPY web-src/pnpm-workspace.yaml web-src/package.json web-src/pnpm-lock.yaml ./web-src/
COPY web-src/packages/navi-ui/package.json ./web-src/packages/navi-ui/
COPY web-src/navi-console/package.json ./web-src/navi-console/
RUN cd web-src && pnpm install --frozen-lockfile

# Build the console frontend (Vite outputs to /src/web via its configured outDir).
COPY web-src/ ./web-src/
RUN cd web-src && pnpm run build

# Stage 2: go build
FROM golang:1.24-alpine AS builder

# Install GCC for CGO (SQLite requirement) and git for go mod download
RUN apk add --no-cache gcc musl-dev git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -buildvcs=false -o /out/navid ./cmd/navid/

# Stage 3: runtime
FROM alpine:3.19

# SQLite libs needed for runtime CGO execution, wget for healthcheck, Python
# for first-party subprocess_python skills, docker-cli for sandbox-backed
# validation via docker exec in the supported Compose runtime, and git for the
# navi-programmer git-lifecycle skill (branch/commit of reviewable output).
RUN apk add --no-cache ca-certificates sqlite-libs wget python3 py3-pip py3-virtualenv docker-cli git

WORKDIR /navi

COPY --from=builder /out/navid .
COPY config/ ./config/
COPY skills/ ./skills/
COPY plugins/ ./plugins/
COPY schema/ ./schema/
# CIP P3 intake extraction/resolution worker (stdlib-only Python; no pip deps).
COPY python/ ./python/
COPY --from=console-builder /src/web ./web

ENTRYPOINT ["./navid"]
