FROM golang:1.15.5-alpine3.12 AS build_base

LABEL Furkan SAYIM @xShuden <furkan.sayim@yandex.com>

RUN apk add --no-cache git

WORKDIR /tmp/app-base

COPY go.mod .
COPY go.sum .
COPY main.go .

RUN go mod download

# Build the Go app
RUN go build -o ./out/urlhunter .

# Start fresh from a smaller image
FROM alpine:3.9 
RUN apk add ca-certificates
RUN apk add xz

COPY --from=build_base /tmp/app-base/out/urlhunter /app/urlhunter

ENTRYPOINT ["/app/urlhunter"]
