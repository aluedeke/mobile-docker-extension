# libimobiledevice Docker image for use with mobile-docker-extension
FROM alpine:3.19

# Install libimobiledevice utilities
RUN apk add --no-cache \
    libimobiledevice \
    libimobiledevice-utils \
    libimobiledevice-progs

# Default to shell for interactive use
CMD ["/bin/sh"]
