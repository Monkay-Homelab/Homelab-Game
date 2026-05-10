#!/bin/sh
set -eu

: "${API_URL:=https://api.homelab.living}"
: "${API_URL_MAP:=}"

cat > /usr/share/nginx/html/config.js <<EOF
window.__APP_CONFIG__ = { API_URL: "${API_URL}", API_URL_MAP: "${API_URL_MAP}" };
EOF

echo "config.js generated with API_URL=${API_URL} API_URL_MAP=${API_URL_MAP}"
