package domain

import "time"

const (
	OrderPending = "PENDING"
	OrderPaid    = "PAID"
	OrderFailed  = "FAILED"
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Product struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SKU        string    `json:"sku"`
	PriceCents int64     `json:"price_cents"`
	Stock      int       `json:"stock"`
	CreatedAt  time.Time `json:"created_at"`
}

type OrderItem struct {
	ProductID     string `json:"product_id"`
	Quantity      int    `json:"quantity"`
	UnitPriceCents int64 `json:"unit_price_cents"`
}

type Order struct {
	ID         string      `json:"id"`
	UserID     string      `json:"user_id"`
	Status     string      `json:"status"`
	TotalCents int64       `json:"total_cents"`
	Items      []OrderItem `json:"items,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type OrderCreatedEvent struct {
	OrderID    string `json:"order_id"`
	UserID     string `json:"user_id"`
	TotalCents int64  `json:"total_cents"`
}
