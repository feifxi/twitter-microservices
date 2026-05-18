FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY apps/shared/ ./apps/shared/
COPY apps/notification-service/ ./apps/notification-service/
WORKDIR /app/apps/notification-service
RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /service ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=builder /service /service
COPY --from=builder /app/apps/notification-service/migrations /migrations
EXPOSE 8080
ENTRYPOINT ["/service"]
