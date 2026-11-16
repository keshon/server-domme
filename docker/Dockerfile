FROM golang:1.26-alpine3.23 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/discord

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /usr/project
COPY --from=builder /out/app ./app
COPY assets ./assets
COPY data/default_task.list.json ./defaults/default_task.list.json
ENV TASKS_PATH=/usr/project/defaults/default_task.list.json
RUN mkdir -p data/store assets/media
# Existing bind-mounted data needs no UID/GID migration.
USER 0:0
ENTRYPOINT ["/usr/project/app"]
