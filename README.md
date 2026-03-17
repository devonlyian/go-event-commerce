# Go Event Commerce (Local Microservices Starter)

Kafka event-driven commerce starter with three services:

- `order-service` (Gin + GORM + PostgreSQL + outbox + Kafka publish)
- `payment-service` (Kafka consume `order.created`, publish payment results)
- `notification-service` (Kafka consume payment events, simulate notification)

## 1. Prerequisites

Installed and verified on local machine:

- Go 1.26.0
- Docker 29.x + Docker Compose v2
- kind 0.31.0
- kubectl v1.35.1
- Helm v4.1.1

## 2. Repository Structure

```
.
|-- services
|   |-- order-service
|   |   |-- cmd/api
|   |   `-- internal/{config,handler,model,repository,service}
|   |-- payment-service
|   |   |-- cmd/worker
|   |   `-- internal/{config,handler,model,repository,service}
|   `-- notification-service
|       |-- cmd/worker
|       `-- internal/{config,handler,model,repository,service}
|-- libs
|   |-- contracts
|   |   |-- events
|   |   `-- topics
|   `-- platform
|       `-- logging
|-- deploy
|   |-- docker
|   |-- kubernetes
|   `-- helm
|-- scripts
|-- docker-compose.yml
|-- Makefile
`-- go.work
```

## 3. Quick Start (Docker Compose)

```bash
make up
```

Wait until containers are healthy, then test:

```bash
curl -s http://localhost:8080/livez
curl -s http://localhost:8080/readyz
curl -s http://localhost:8081/readyz
curl -s http://localhost:8082/readyz
```

Open local UIs:

- Order API Swagger UI: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- Kafka UI: [http://localhost:8088](http://localhost:8088)

Create order:

```bash
curl -s -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"customer-001","amount":129.99}'
```

Get order (replace `<order_id>`):

```bash
curl -s http://localhost:8080/orders/<order_id>
```

`POST /orders` may return `202 Accepted` if the order was persisted but the initial Kafka publish failed.
In that case the outbox relay retries publishing in the background, and the order status will still change asynchronously to `paid` or `payment_failed`.

Smoke test the full flow:

```bash
bash scripts/run-curl-demo.sh
```

Stop:

```bash
make down
```

## 4. Kafka Topics

Created automatically by `kafka-init` service:

- `order.created`
- `payment.completed`
- `payment.failed`

Manual creation:

```bash
./scripts/create-kafka-topics.sh
```

## 5. Build/Test

```bash
make build
make test
```

Generate Swagger docs:

```bash
make swagger
```

## 6. Kubernetes (kind)

Kubernetes manifests in this starter run microservices on kind and reuse Kafka/PostgreSQL from Docker Compose via `host.docker.internal`.
Keep `make up` running before applying Kubernetes manifests.

Create cluster (if needed):

```bash
make kind-up
```

Apply manifests (build images + load to kind + apply resources):

```bash
make k8s-apply
```

Check pods:

```bash
kubectl -n commerce get pods
```

Test order-service from kind:

```bash
kubectl -n commerce port-forward svc/order-service 18080:8080
curl -s http://localhost:18080/readyz
```

Delete manifests:

```bash
make k8s-delete
```

Delete cluster:

```bash
make kind-down
```

## 7. Service Responsibilities

### order-service

- `POST /orders`
- `GET /orders/:id`
- `GET /livez`
- `GET /readyz`
- `GET /swagger/index.html`
- persists order in PostgreSQL
- writes event to outbox table
- publishes `order.created`
- reflects payment result events back into order status

### payment-service

- consumes `order.created`
- simulates payment processing
- publishes `payment.completed` or `payment.failed`

### notification-service

- consumes payment result events
- logs simulated notification dispatch

## 8. Notes

- This starter intentionally keeps domain scope to order/payment/notification only.
- If you want to add extra business capabilities, discuss first before implementation.
- Shared modules:
  - `libs/contracts`: event payload contracts and topic names
  - `libs/platform`: shared platform utilities (currently structured logger builder)
