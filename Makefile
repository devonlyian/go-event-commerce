SHELL := /bin/bash
COMPOSE := docker compose
CLUSTER_NAME ?= commerce-dev

.PHONY: up down restart logs ps test build swagger kind-up kind-down kind-load-images k8s-apply k8s-delete topics

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down -v

restart: down up

logs:
	$(COMPOSE) logs -f --tail=200

ps:
	$(COMPOSE) ps

topics:
	$(COMPOSE) run --rm kafka-init

build:
	cd services/order-service && go build ./...
	cd services/payment-service && go build ./...
	cd services/notification-service && go build ./...

test:
	cd services/order-service && go test ./...
	cd services/payment-service && go test ./...
	cd services/notification-service && go test ./...

swagger:
	cd services/order-service && go run github.com/swaggo/swag/cmd/swag@latest init -g main.go -d cmd/api,internal/handler,internal/model -o docs --parseInternal

kind-up:
	kind get clusters | grep -q "^$(CLUSTER_NAME)$$" || kind create cluster --name $(CLUSTER_NAME)

kind-down:
	kind delete cluster --name $(CLUSTER_NAME)

kind-load-images:
	docker build -f deploy/docker/order-service.Dockerfile -t order-service:dev .
	docker build -f deploy/docker/payment-service.Dockerfile -t payment-service:dev .
	docker build -f deploy/docker/notification-service.Dockerfile -t notification-service:dev .
	kind load docker-image order-service:dev payment-service:dev notification-service:dev --name $(CLUSTER_NAME)

k8s-apply: kind-load-images
	kubectl apply -f deploy/kubernetes/namespace.yaml
	kubectl apply -f deploy/kubernetes/order-service.yaml
	kubectl apply -f deploy/kubernetes/payment-service.yaml
	kubectl apply -f deploy/kubernetes/notification-service.yaml
	kubectl -n commerce rollout restart deployment/order-service deployment/payment-service deployment/notification-service
	kubectl -n commerce rollout status deployment/order-service --timeout=120s
	kubectl -n commerce rollout status deployment/payment-service --timeout=120s
	kubectl -n commerce rollout status deployment/notification-service --timeout=120s

k8s-delete:
	kubectl delete -f deploy/kubernetes/notification-service.yaml --ignore-not-found
	kubectl delete -f deploy/kubernetes/payment-service.yaml --ignore-not-found
	kubectl delete -f deploy/kubernetes/order-service.yaml --ignore-not-found
	kubectl delete -f deploy/kubernetes/namespace.yaml --ignore-not-found
