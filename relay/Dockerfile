# syntax=docker/dockerfile:1.7

# ============================================================================
# Stage 1: builder
#   - Pinned to golang:1.23.2-alpine by SHA digest (scripts/check-pins.sh
#     gate rejects any non-SHA reference).
#   - BuildKit cache mounts speed up `go mod download` + `go build` on CI repeats.
#   - CGO disabled + -trimpath + -ldflags='-s -w' produces a static, stripped
#     binary suitable for distroless/static (~10 MB).
# ============================================================================
FROM --platform=$BUILDPLATFORM golang:1.23.2-alpine@sha256:9dd2625a1ff2859b8d8b01d8f7822c0f528942fe56cfe7a1e7c38d3b8d72d679 AS builder

# ARGs for cross-compilation: BuildKit sets these per --platform target.
ARG TARGETOS
ARG TARGETARCH
# Version/commit/buildTime labels for OCI metadata + the binary's --version flag.
# Populated by goreleaser via build_flag_templates (see relay/.goreleaser.yaml).
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown

WORKDIR /src

# Copy module manifests first so `go mod download` is cacheable across source-only changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the relay source tree (cmd/, internal/, etc.).
COPY . .

# Build the relay binary. Resolution 2: -X targets the lowercase package-level
# vars (version, commit, buildTime) — the canonical buildinfo pattern. Go's
# linker -X flag works on unexported vars.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -ldflags="-s -w -X intake/internal/version.version=${VERSION} -X intake/internal/version.commit=${COMMIT} -X intake/internal/version.buildTime=${BUILD_TIME}" \
      -o /out/intake-relay \
      ./cmd/relay

# 7-iii dependency: build fake-llm into the same image so the docker-compose
# demo can reuse one image with a command-override (`entrypoint: []` +
# `command: [/fake-llm, …]` on the fake-llm service).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/fake-llm \
      ./cmd/fake-llm

# ============================================================================
# Stage 2: runner (distroless/static-debian12:nonroot)
#   - SHA-pinned; gate in scripts/check-pins.sh rejects any non-SHA reference.
#   - distroless/static contains no shell, no package manager, no apt CVEs.
#   - The nonroot variant defaults USER to 65532:65532.
#   - Final image total: < 50 MB (enforced as build-fail row).
# ============================================================================
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

# OCI image annotations (operator/registry visible metadata).
ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/intake/intake" \
      org.opencontainers.image.description="Intake relay — Go HTTP server for the intake widget" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

# Copy the static binaries from the builder stage. Distroless ships /etc/passwd
# with `nonroot` mapped to UID 65532; preserve that ownership.
COPY --from=builder --chown=nonroot:nonroot /out/intake-relay /intake-relay
# 7-iii: fake-llm binary present in the same image, overridden by the
# fake-llm service in docker-compose via `entrypoint: []` + `command:`.
COPY --from=builder --chown=nonroot:nonroot /out/fake-llm /fake-llm

# Distroless/static ships USER=nonroot (UID 65532) — restate it explicitly to
# satisfy auditors and the `docker inspect --format '{{.Config.User}}'` invariant.
USER nonroot:nonroot

# 8080 — main relay HTTP server (config.server.listen)
# 9090 — Prometheus metrics endpoint (config.observability.metrics.addr)
EXPOSE 8080 9090

ENTRYPOINT ["/intake-relay"]
CMD ["--config", "/etc/intake/config.yaml"]
