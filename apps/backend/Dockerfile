# syntax=docker/dockerfile:1.7

# Build on the native platform and cross-compile for the target one, so
# images work on amd64 and arm64 hosts (e.g. Apple Silicon) alike.
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Digolitus Status Page Backend" \
      org.opencontainers.image.description="Backend API for Digolitus Status Page" \
      org.opencontainers.image.source="https://github.com/digolitus/digolitus-statuspage"

COPY --from=builder /out/api /api
COPY --from=builder /out/healthcheck /healthcheck

USER nonroot:nonroot

EXPOSE 8080

# "serve" (default) runs the API; "migrate" applies migrations and exits.
ENTRYPOINT ["/api"]
