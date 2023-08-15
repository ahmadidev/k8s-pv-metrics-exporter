# syntax = docker/dockerfile:1-experimental
# https://www.docker.com/blog/containerize-your-go-developer-environment-part-2/

FROM golang:1.19-alpine3.15 AS build
WORKDIR /app
ENV https_proxy=http://infra:infra88@192.168.0.182:10811
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o app

FROM alpine:3.15
COPY --from=build /app/app /app/app
ENTRYPOINT ["/app/app"]