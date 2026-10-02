package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all environment-driven settings for the service.
// Phase 3 adds Kafka brokers and the payment simulator's failure rate.
type Config struct {
	Port        string
	DatabaseURL string
	RedisAddr   string

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	KafkaBrokers       []string
	PaymentFailureRate float64

	AdminEmail    string
	AdminPassword string
	CORSOrigin    string

	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string

	TwilioAccountSID          string
	TwilioAuthToken           string
	TwilioFromNumber          string // SMS-capable Twilio number, e.g. +15017122661
	TwilioMessagingServiceSID string // preferred for trial accounts / approved templates
	TwilioDefaultCountryCode  string // prepended to phone numbers that don't already start with '+', e.g. +91

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	HolidayCalendarURL       string
	HolidayCalendarProvider  string
	CalendarificAPIKey       string
	HolidayCountryCode       string
	FestivalTimezone         string
	FestivalDiscountPercent  int
	FestivalStockPerProduct  int
	FestivalProductsPerEvent int
	VAPIDPublicKey           string
	VAPIDPrivateKey          string
	VAPIDSubject             string
	GroqAPIKey               string
	GroqModel                string
}

func Load() Config {
	return Config{
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://flashcart:flashcart@localhost:5432/flashcart?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		JWTSecret:          getEnv("JWT_SECRET", "dev-secret-change-me"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		KafkaBrokers:       strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		PaymentFailureRate: getEnvFloat("PAYMENT_FAILURE_RATE", 0.1),
		AdminEmail:         getEnv("ADMIN_EMAIL", "admin@flashcart.local"),
		AdminPassword:      getEnv("ADMIN_PASSWORD", "Admin@12345"),
		CORSOrigin:         getEnv("CORS_ORIGIN", "*"),

		RazorpayKeyID:         getEnv("RAZORPAY_KEY_ID", ""),
		RazorpayKeySecret:     getEnv("RAZORPAY_KEY_SECRET", ""),
		RazorpayWebhookSecret: getEnv("RAZORPAY_WEBHOOK_SECRET", ""),

		TwilioAccountSID:          getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:           getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioFromNumber:          getEnv("TWILIO_FROM_NUMBER", ""),
		TwilioMessagingServiceSID: getEnv("TWILIO_MESSAGING_SERVICE_SID", ""),
		TwilioDefaultCountryCode:  getEnv("TWILIO_DEFAULT_COUNTRY_CODE", "+91"),
		SMTPHost:                  getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnv("SMTP_PORT", "587"),
		SMTPUsername:              getEnv("SMTP_USERNAME", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getEnv("SMTP_FROM", ""),
		HolidayCalendarURL:        getEnv("HOLIDAY_CALENDAR_URL", ""),
		HolidayCalendarProvider:   getEnv("HOLIDAY_CALENDAR_PROVIDER", "calendarific"),
		CalendarificAPIKey:        getEnv("CALENDARIFIC_API_KEY", ""),
		HolidayCountryCode:        getEnv("HOLIDAY_COUNTRY_CODE", "IN"),
		FestivalTimezone:          getEnv("FESTIVAL_TIMEZONE", "Asia/Kolkata"),
		FestivalDiscountPercent:   getEnvInt("FESTIVAL_DISCOUNT_PERCENT", 10),
		FestivalStockPerProduct:   getEnvInt("FESTIVAL_STOCK_PER_PRODUCT", 10),
		FestivalProductsPerEvent:  getEnvInt("FESTIVAL_PRODUCTS_PER_EVENT", 20),
		VAPIDPublicKey:            getEnv("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey:           getEnv("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:              getEnv("VAPID_SUBJECT", "mailto:admin@flashcart.local"),
		GroqAPIKey:                getEnv("GROQ_API_KEY", ""),
		GroqModel:                 getEnv("GROQ_MODEL", "openai/gpt-oss-120b"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}
