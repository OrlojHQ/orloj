# syntax=docker/dockerfile:1

# --- Frontend (only required for orlojd embed) ---
FROM oven/bun:1.3-alpine AS ui
WORKDIR /frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# --- Go module cache ---
FROM golang:1.25-alpine AS base
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

# --- Runtime images (default final stage: orlojd) ---
FROM alpine:3.20 AS orlojworker
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser
COPY --from=build-orlojworker /out/orlojworker /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]

FROM alpine:3.20 AS orlojd
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 appuser
COPY --from=build-orlojd /out/orlojd /usr/local/bin/app
USER appuser
ENTRYPOINT ["/usr/local/bin/app"]
