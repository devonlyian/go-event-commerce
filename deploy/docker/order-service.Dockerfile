FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY libs ./libs
COPY services ./services
WORKDIR /src/services/order-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/order-service ./cmd/api

FROM gcr.io/distroless/base-debian12
COPY --from=builder /bin/order-service /order-service
EXPOSE 8080
ENTRYPOINT ["/order-service"]
