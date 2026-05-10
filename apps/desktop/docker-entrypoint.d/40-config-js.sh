#!/bin/sh
set -eu

: "${API_URL:=https://api.homelab.living}"

cat > /usr/share/nginx/html/config.js <<EOF
window.__APP_CONFIG__ = { API_URL: "${API_URL}" };
EOF

echo "config.js generated with API_URL=${API_URL}"
