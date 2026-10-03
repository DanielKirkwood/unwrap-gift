# Dev-only image for deploy/dev/docker-compose.yml: rebuilds and restarts
# unwrap-gift via air whenever a .go file under the bind-mounted repo root
# changes. Not used in production or in any other compose file — see
# ../../Dockerfile for the real, static build.
FROM golang:1.27-alpine

RUN apk add --no-cache ca-certificates git && \
    go install github.com/air-verse/air@latest

WORKDIR /app

# Source (including go.mod/go.sum) arrives via the bind mount in
# docker-compose.yml, not COPY — this image has no code of its own baked in,
# so rebuilding it is only ever needed if this Dockerfile itself changes.
EXPOSE 8080 8081 8082 8083

ENTRYPOINT ["air", "-c", ".air.toml"]
