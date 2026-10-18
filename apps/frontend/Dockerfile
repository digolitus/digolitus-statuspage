# syntax=docker/dockerfile:1.7

FROM node:22.12.0-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.27.3-alpine

LABEL org.opencontainers.image.title="Digolitus Status Page Frontend"
LABEL org.opencontainers.image.description="Frontend for Digolitus Status Page"

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080

USER 101

CMD ["nginx", "-g", "daemon off;"]
