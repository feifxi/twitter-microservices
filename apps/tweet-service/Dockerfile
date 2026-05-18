FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY apps/shared/ ./apps/shared/
COPY apps/tweet-service/ ./apps/tweet-service/
WORKDIR /app/apps/tweet-service
RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /service ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=builder /service /service
COPY --from=builder /app/apps/tweet-service/migrations /migrations
EXPOSE 8080
ENTRYPOINT ["/service"]
