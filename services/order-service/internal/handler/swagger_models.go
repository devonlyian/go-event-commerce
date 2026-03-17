package handler

import "github.com/devonlyian/go-event-commerce/services/order-service/internal/model"

type CreateOrderRequestDoc struct {
	CustomerID string  `json:"customer_id" example:"customer-001"`
	Amount     float64 `json:"amount" example:"129.99"`
}

type OrderResponseDoc struct {
	Order model.Order `json:"order"`
}

type OrderAcceptedResponseDoc struct {
	Order   model.Order `json:"order"`
	Warning string      `json:"warning" example:"order persisted but event publish failed"`
}

type ErrorResponseDoc struct {
	Error  string `json:"error" example:"invalid request"`
	Detail string `json:"detail,omitempty" example:"Key: 'CreateOrderRequestDoc.Amount' Error:Field validation for 'Amount' failed on the 'gt' tag"`
}

type HealthResponseDoc struct {
	Status string `json:"status" example:"ok"`
}

type ReadinessFailureResponseDoc struct {
	Status string `json:"status" example:"not ready"`
	Detail string `json:"detail" example:"dial tcp 127.0.0.1:5432: connect: connection refused"`
}
