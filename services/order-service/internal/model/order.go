package model

import "time"

type Order struct {
	ID         string    `gorm:"type:uuid;primaryKey" json:"id"`
	CustomerID string    `gorm:"size:64;not null" json:"customer_id"`
	Amount     float64   `gorm:"type:numeric(12,2);not null" json:"amount"`
	Status     string    `gorm:"size:32;not null" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
