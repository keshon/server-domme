# ---- Stage 1: Builder ----
FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://proxy.golang.org,direct

RUN apk add --no-cache git

WORKDIR /usr/project

# Cache dependencies layer
COPY src/go.mod src/go.sum ./
RUN go mod download

# Copy rest of the source
COPY src .

# Build with embedded metadata
RUN BUILD_DATE=$(date -u +"%Y-%m-%dT%H-%M-%SZ") && \
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none") && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
        -o app \
        -ldflags="-s -w \
            -X github.com/keshon/buildinfo.Version=dev \
            -X github.com/keshon/buildinfo.Commit=${GIT_COMMIT} \
            -X github.com/keshon/buildinfo.BuildTime=${BUILD_DATE} \
            -X 'github.com/keshon/buildinfo.Project=Server Domme' \
            -X 'github.com/keshon/buildinfo.Description=Discord bot for server management, roleplay tasks, confessions and scheduled channel cleanup.'" \
        ./cmd/discord/

# ---- Stage 2: Runtime ----
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /usr/project
COPY --from=builder /usr/project/app ./app

# The datastore creates and owns its directory; it must not be pre-seeded
# with files, only mounted.
RUN mkdir -p data

ENTRYPOINT ["/usr/project/app"]
