# syntax=docker/dockerfile:1

# ---- builder ----
# Alpine, not because of cgo (modernc.org/sqlite is pure Go — no cgo,
# nothing to link against libc for), just for a smaller builder image.
FROM golang:1.27-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# VERSION is passed by CI/the Taskfile (git describe/SHA); defaults to
# "dev" for a plain `docker build .` with nothing supplied. See
# internal/app.Version's doc comment for why this exists instead of
# relying on debug.ReadBuildInfo() alone.
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/DanielKirkwood/unwrap-gift/internal/app.Version=${VERSION}" \
    -o /out/unwrap-gift .

# A fresh named volume mounted at /data (see docker-compose.yml,
# deploy/production/docker-compose.yml) is initialized by Docker from
# whatever's already at that path in the image, ownership included — scratch
# has no shell to chown it at runtime, so it has to arrive pre-owned by the
# non-root user below.
RUN mkdir -p /data-empty && chown 65532:65532 /data-empty

# ---- final ----
# scratch, not distroless/alpine: the binary is fully static (CGO_ENABLED=0,
# pure-Go sqlite driver) and needs nothing from a base image except CA
# certs for outbound TLS (OTLP exporter, SMTP-over-TLS via Kratos).
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/unwrap-gift /unwrap-gift
COPY --from=builder --chown=65532:65532 /data-empty /data

# Public/protected/hidden — see internal/app/servers.go. Compose files
# decide which of these actually get published; scratch has no shell to
# restrict this further at the image level.
EXPOSE 8080 8081 8082

# Numeric UID: scratch has no /etc/passwd, but the kernel only needs a
# number — this binary never calls os/user.Lookup.
USER 65532:65532

ENTRYPOINT ["/unwrap-gift"]
CMD ["start"]
