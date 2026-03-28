FROM ghcr.io/joojoooo/immich-upload-optimizer:latest AS runtime
RUN apt-get update && apt-get upgrade -y && apt-get clean && rm -rf /var/lib/apt/lists/*

FROM golang:1.25-alpine AS builder
ARG VERSION=patched
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /immich-upload-optimizer .

FROM runtime
COPY --from=builder /immich-upload-optimizer /usr/local/bin/immich-upload-optimizer
ENTRYPOINT ["immich-upload-optimizer"]
