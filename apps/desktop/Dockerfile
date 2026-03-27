# ---- Build stage ----
FROM node:22-alpine AS builder

RUN corepack enable

WORKDIR /app

# Copy workspace root files
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./

# Copy package.json files for dependency resolution
COPY packages/shared/package.json packages/shared/package.json
COPY apps/desktop/package.json apps/desktop/package.json

RUN pnpm install --frozen-lockfile

# Copy source
COPY packages/shared/ packages/shared/
COPY apps/desktop/ apps/desktop/

# Build-time API URL (defaults to production)
ARG VITE_API_URL=https://api.homelab.living
ENV VITE_API_URL=$VITE_API_URL

RUN cd apps/desktop && pnpm build

# ---- Runtime stage ----
FROM nginx:1.27-alpine

COPY --from=builder /app/apps/desktop/dist /usr/share/nginx/html
COPY apps/desktop/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --spider -q http://localhost/ || exit 1
