# ── Stage 1: Builder ─────────────────────────────────────────────────────────
# Use the official Go image to compile the binary.
# All build tools live here; the runtime image receives only the final binary.
FROM golang:1.25-bookworm AS builder

WORKDIR /build

# Cache go module downloads in a separate layer.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary.
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w" \
    -o /build/cnpg-ui \
    ./cmd/cnpg-ui

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
# Use Google's distroless/static-debian12 as the runtime base.
# It contains only the minimum OS libraries needed to run a statically linked binary.
# No shell, no package manager — minimal attack surface.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

# Copy only the binary from the builder stage.
COPY --from=builder /build/cnpg-ui /cnpg-ui

# Expose the default HTTP port.
EXPOSE 8080

# Use the built-in nonroot user from distroless (uid 65532).
USER 65532:65532

ENTRYPOINT ["/cnpg-ui"]
