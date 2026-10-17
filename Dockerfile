# syntax=docker/dockerfile:1

# Build a static binary for the target platform (linux/arm64 in production).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# Distroless: no shell or package manager, runs as a non-root user. The binary
# also embeds the tz database (internal/raid imports time/tzdata), so time
# zones work even on a base image without /usr/share/zoneinfo.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/bot /bot
ENTRYPOINT ["/bot"]
