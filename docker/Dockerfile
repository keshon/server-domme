# ---- Stage 1: Builder ----
FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://proxy.golang.org,direct

RUN apk add --no-cache git

WORKDIR /usr/project

# Cache dependencies layer
COPY src/go.mod src/go.sum ./
COPY src/pkg ./pkg
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
            -X github.com/keshon/buildinfo.Project=ServerDomme \
            -X 'github.com/keshon/buildinfo.Description=Server Domme'" \
        ./cmd/discord/

# ---- Stage 2: Runtime ----
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /usr/project
COPY --from=builder /usr/project/app ./app

RUN mkdir -p data && touch data/datastore.json

ENTRYPOINT ["/usr/project/app"]
