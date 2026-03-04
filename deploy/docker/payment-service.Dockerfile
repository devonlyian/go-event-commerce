FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY services/payment-service/go.mod services/payment-service/go.sum ./services/payment-service/
WORKDIR /src/services/payment-service
RUN go mod download

COPY services/payment-service ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/payment-service ./cmd/worker

FROM gcr.io/distroless/base-debian12
COPY --from=builder /bin/payment-service /payment-service
EXPOSE 8081
ENTRYPOINT ["/payment-service"]
