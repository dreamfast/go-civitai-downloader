#!/bin/bash
set -e

# Cleanup function
cleanup() {
  echo "Received exit signal. Cleaning up..."
  kill -TERM "$NGINX_PID" "${DL_PIDS[@]}" 2>/dev/null
  wait "$NGINX_PID" "${DL_PIDS[@]}"
  exit 0
}

# Trap termination signals
trap cleanup SIGINT SIGTERM

# Create the output folder
mkdir /workspace/civitai-export

# Interpolate the config
envsubst < /etc/civitai/config.template.toml > /etc/civitai/config.toml

# Start nginx
nginx -g 'daemon off;' &
NGINX_PID=$!

# Split usernames by comma
IFS=',' read -ra USERNAMES <<< "$CIVITAI_USERNAME"

# Keep track of background download processes
DL_PIDS=()

# Start a download process for each username
for username in "${USERNAMES[@]}"; do
  echo "Starting download for user: $username"

  (
    /usr/bin/civitai-downloader download -u "$username" -c 4 --save-model-info -y --config /etc/civitai/config.toml
    /usr/bin/civitai-downloader images -u "$username" -c 4 --save-metadata --config /etc/civitai/config.toml
  ) &
  DL_PIDS+=($!)
done

# Wait for all background processes
wait "$NGINX_PID" "${DL_PIDS[@]}"
