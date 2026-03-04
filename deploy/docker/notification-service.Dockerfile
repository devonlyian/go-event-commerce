FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY services/notification-service/go.mod services/notification-service/go.sum ./services/notification-service/
WORKDIR /src/services/notification-service
RUN go mod download

COPY services/notification-service ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/notification-service ./cmd/worker

FROM gcr.io/distroless/base-debian12
COPY --from=builder /bin/notification-service /notification-service
EXPOSE 8082
ENTRYPOINT ["/notification-service"]
