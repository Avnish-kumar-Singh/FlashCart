package events

import "time"

const (
	TopicOrderCreated   = "order.created"
	TopicOrderConfirmed = "order.confirmed"
	TopicOrderCancelled = "order.cancelled"
	TopicFestivalNotice = "festival.notice"

	SourceCart      = "CART"
	SourceFlashSale = "FLASH_SALE"
)

type OrderItemPayload struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type OrderCreated struct {
	OrderID       string             `json:"order_id"`
	UserID        string             `json:"user_id"`
	TotalPaise    int64              `json:"total_paise"`
	Items         []OrderItemPayload `json:"items"`
	CreatedAt     time.Time          `json:"created_at"`
	Source        string             `json:"source"`
	FlashSaleID   string             `json:"flash_sale_id,omitempty"`
	ReservationID string             `json:"reservation_id,omitempty"`
}

type OrderConfirmed struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

type OrderCancelled struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Reason  string `json:"reason"`
}

type FestivalNotice struct {
	FestivalID      string    `json:"festival_id"`
	Name            string    `json:"name"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	DiscountPercent int       `json:"discount_percent"`
	Phase           string    `json:"phase"`
}
