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
mkdir -p /workspace/civitai-export 

# Interpolate the config
envsubst < /etc/civitai/config.template.toml > /etc/civitai/config.toml

# Start nginx
nginx -g 'daemon off;' &
NGINX_PID=$!

# Split usernames by comma
IFS=',' read -ra USERNAMES <<< "$CIVITAI_USERNAME"

# Keep track of background download processes
DL_PIDS=()

# Sequentially process each user
for username in "${USERNAMES[@]}"; do
  echo "Starting download for user: $username"

  /usr/bin/civitai-downloader download -u "$username" -c 4 --model-info -y --config /etc/civitai/config.toml
  /usr/bin/civitai-downloader images -u "$username" -c 4 --metadata --config /etc/civitai/config.toml

done &

DL_PID=$!

# Wait for nginx and the download loop
wait "$NGINX_PID" "$DL_PID"
