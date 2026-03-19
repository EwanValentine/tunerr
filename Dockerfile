# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /tunerr .

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM scratch

# CA certs for HTTPS calls to Lidarr (or any TLS endpoint).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /tunerr /tunerr

# Volumes are declared here for documentation; actual mounts go in compose.
VOLUME ["/downloads", "/music"]

ENTRYPOINT ["/tunerr"]
