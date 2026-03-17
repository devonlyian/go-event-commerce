FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY libs ./libs
COPY services ./services
WORKDIR /src/services/notification-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/notification-service ./cmd/worker

FROM gcr.io/distroless/base-debian12
COPY --from=builder /bin/notification-service /notification-service
EXPOSE 8082
ENTRYPOINT ["/notification-service"]
