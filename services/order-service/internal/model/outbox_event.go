package model

import "time"

type OutboxEvent struct {
	ID            string     `gorm:"type:uuid;primaryKey" json:"id"`
	AggregateID   string     `gorm:"type:uuid;index;not null" json:"aggregate_id"`
	AggregateType string     `gorm:"size:64;index;not null" json:"aggregate_type"`
	Topic         string     `gorm:"size:255;index;not null" json:"topic"`
	Payload       []byte     `gorm:"type:jsonb;not null" json:"payload"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	PublishError  string     `gorm:"type:text" json:"publish_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
