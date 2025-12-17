# go-ios Docker image for use with usbmuxd-docker-extension
FROM golang:1.22-alpine AS builder
RUN go install github.com/danielpaulus/go-ios@latest

FROM alpine:3.19
COPY --from=builder /go/bin/go-ios /usr/local/bin/ios
RUN apk add --no-cache ca-certificates

ENTRYPOINT ["ios"]
