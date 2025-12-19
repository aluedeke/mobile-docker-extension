# pymobiledevice3 Docker image for use with mobile-docker-extension
FROM python:3.12-slim

# Install build dependencies for sslpsk-pmd3
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir pymobiledevice3

ENTRYPOINT ["pymobiledevice3"]
