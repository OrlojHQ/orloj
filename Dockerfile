# syntax=docker/dockerfile:1

# --- Frontend (only required for orlojd embed) ---
FROM oven/bun:1.3-alpine AS ui
WORKDIR /frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# --- Go module cache ---
FROM golang:1.26.4-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS base
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
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS orlojworker
RUN apk add --no-cache ca-certificates tzdata wget docker-cli \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-orlojworker /out/orlojworker /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]

FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS orloj-operator
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-operator /out/orloj-operator /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]

FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS orlojd
RUN apk add --no-cache ca-certificates tzdata wget docker-cli \
    && adduser -D -u 10001 appuser
COPY --from=orloj-legal /usr/share/doc/orloj /usr/share/doc/orloj
COPY --from=build-orlojd /out/orlojd /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]
