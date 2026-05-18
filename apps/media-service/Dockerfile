FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY apps/shared/ ./apps/shared/
COPY apps/media-service/ ./apps/media-service/
WORKDIR /app/apps/media-service
RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /service ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=builder /service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
