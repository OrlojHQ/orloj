FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BINARY=orlojd
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/${BINARY} ./cmd/${BINARY}

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser

ARG BINARY=orlojd
COPY --from=builder /out/${BINARY} /usr/local/bin/app

USER appuser

ENTRYPOINT ["/usr/local/bin/app"]
