# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/proxy-service .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/proxy-service ./proxy-service
USER app:app
EXPOSE 8000
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:8000/health || exit 1
ENTRYPOINT ["/app/proxy-service"]
