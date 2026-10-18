# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:26.12.0-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.31.7-alpine

LABEL org.opencontainers.image.title="Digolitus Status Page Frontend" \
      org.opencontainers.image.description="Frontend for Digolitus Status Page" \
      org.opencontainers.image.source="https://github.com/digolitus/digolitus-statuspage"

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

USER 101

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
