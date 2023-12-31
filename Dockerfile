# syntax = docker/dockerfile:1-experimental
# https://www.docker.com/blog/containerize-your-go-developer-environment-part-2/

FROM golang:1.19-alpine3.15 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o app

FROM alpine:3.15
LABEL org.opencontainers.image.source=https://github.com/ahmadidev/k8s-pv-metrics-exporter
LABEL org.opencontainers.image.description="A Prometheus exporter for Kubernetes Persistent Volumes"
COPY --from=build /app/app /app/app
ENTRYPOINT ["/app/app"]
