# Use an official Golang image for building
FROM golang:1.24-bullseye AS builder

# Build the civitai-downloader
RUN git clone https://github.com/dreamfast/go-civitai-downloader.git /src \
    && cd /src \
    && make build

# Final runtime image
FROM debian:bullseye-slim

# Install nginx and any required dependencies
RUN apt-get update && apt-get install -y nginx ca-certificates gettext-base && apt-get clean

# Copy civitai-downloader binary
COPY --from=builder /src/civitai-downloader /usr/bin/civitai-downloader

# Copy configs and entrypoint last
COPY docker/nginx.conf /etc/nginx/nginx.conf
COPY docker/civitai-config.template.toml /etc/civitai/config.template.toml
COPY docker/entrypoint.sh /entrypoint.sh

# Set permissions
RUN chmod +x /entrypoint.sh

# Expose nginx port
EXPOSE 80

# Entrypoint
ENTRYPOINT ["/entrypoint.sh"]

