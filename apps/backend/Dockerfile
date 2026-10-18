# syntax=docker/dockerfile:1.7

FROM golang:1.23.2-bookworm AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Digolitus Status Page Backend"
LABEL org.opencontainers.image.description="Backend API for Digolitus Status Page"

COPY --from=builder /out/api /api
COPY --from=builder /out/healthcheck /healthcheck

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/api"]
