# go-ios Docker image for use with usbmuxd-docker-extension
FROM golang:1.22-alpine AS builder
RUN go install github.com/danielpaulus/go-ios@latest

FROM alpine:3.19
COPY --from=builder /go/bin/go-ios /usr/local/bin/ios
# iproute2 needed for TUN device configuration (ip command)
RUN apk add --no-cache ca-certificates iproute2

# go-ios uses macOS-style ifconfig commands, but Alpine uses BusyBox ifconfig
# This wrapper translates macOS ifconfig syntax to Linux ip commands
COPY ifconfig-wrapper.sh /usr/sbin/ifconfig
RUN chmod +x /usr/sbin/ifconfig

ENTRYPOINT ["ios"]
