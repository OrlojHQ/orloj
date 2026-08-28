# syntax=docker/dockerfile:1

# --- Frontend (only required for orlojd embed) ---
FROM oven/bun:1.4-alpine AS ui
WORKDIR /frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# --- Go module cache ---
FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# --- orlojd binary (embeds frontend/dist) ---
FROM base AS build-orlojd
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
COPY --from=ui /frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-s -w -X github.com/OrlojHQ/orloj/internal/version.Version=${VERSION} -X github.com/OrlojHQ/orloj/internal/version.Commit=${COMMIT} -X github.com/OrlojHQ/orloj/internal/version.Date=${DATE}" \
    -o /out/orlojd ./cmd/orlojd

# --- orlojworker binary (no UI) ---
FROM base AS build-orlojworker
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-s -w -X github.com/OrlojHQ/orloj/internal/version.Version=${VERSION} -X github.com/OrlojHQ/orloj/internal/version.Commit=${COMMIT} -X github.com/OrlojHQ/orloj/internal/version.Date=${DATE}" \
    -o /out/orlojworker ./cmd/orlojworker

# --- orloj-operator binary (no UI, no docker-cli) ---
FROM base AS build-operator
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-s -w -X github.com/OrlojHQ/orloj/internal/version.Version=${VERSION} -X github.com/OrlojHQ/orloj/internal/version.Commit=${COMMIT} -X github.com/OrlojHQ/orloj/internal/version.Date=${DATE}" \
    -o /out/orloj-operator ./cmd/orloj-operator

# --- Legal/attribution docs bundled in runtime images ---
FROM scratch AS orloj-legal
COPY LICENSE NOTICE TRADEMARKS.md /usr/share/doc/orloj/

# --- Runtime images (default final stage: orlojd) ---
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS orlojworker
RUN apk add --no-cache ca-certificates tzdata wget docker-cli \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-orlojworker /out/orlojworker /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS orloj-operator
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-operator /out/orloj-operator /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS orlojd
RUN apk add --no-cache ca-certificates tzdata wget docker-cli \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-orlojd /out/orlojd /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]
