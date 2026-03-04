package events

import "time"

type OrderCreatedEvent struct {
	EventID    string    `json:"event_id"`
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	Amount     float64   `json:"amount"`
	OccurredAt time.Time `json:"occurred_at"`
}

type PaymentEvent struct {
	EventID    string    `json:"event_id"`
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	Amount     float64   `json:"amount"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}
