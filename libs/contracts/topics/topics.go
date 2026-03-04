package topics

const (
	OrderCreated     = "order.created"
	PaymentCompleted = "payment.completed"
	PaymentFailed    = "payment.failed"
)

func PaymentTopics() []string {
	return []string{PaymentCompleted, PaymentFailed}
}
