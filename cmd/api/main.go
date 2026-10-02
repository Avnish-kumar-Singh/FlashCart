package main

import (
	"log"
	"net/http"

	"github.com/yourname/flashcart/internal/auth"
	"github.com/yourname/flashcart/internal/cache"
	"github.com/yourname/flashcart/internal/config"
	"github.com/yourname/flashcart/internal/db"
	"github.com/yourname/flashcart/internal/handlers"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/notify"
)

func main() {
	cfg := config.Load()

	pool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	if err := db.EnsureAdmin(pool, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("admin bootstrap failed: %v", err)
	}

	redisClient, err := cache.NewClient(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer redisClient.Close()

	producer := kafka.NewProducer(cfg.KafkaBrokers)
	defer producer.Close()

	issuer := auth.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	// Real SMS via Twilio when credentials are configured; otherwise fall
	// back to logging, exactly like the payment simulator falls back when
	// Razorpay keys are absent. This is what actually sends OTPs/order
	// updates to a phone instead of just printing them to the container log.
	var notifier notify.Sender
	if cfg.TwilioAccountSID != "" && cfg.TwilioAuthToken != "" && (cfg.TwilioFromNumber != "" || cfg.TwilioMessagingServiceSID != "") {
		log.Println("notify: using Twilio for SMS (TWILIO_ACCOUNT_SID set)")
		notifier = notify.NewTwilioSender(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioFromNumber, cfg.TwilioDefaultCountryCode, cfg.TwilioMessagingServiceSID, notify.NewLogSender())
	} else {
		log.Println("notify: TWILIO_ACCOUNT_SID/TWILIO_AUTH_TOKEN/TWILIO_FROM_NUMBER or TWILIO_MESSAGING_SERVICE_SID not fully set — SMS will only be logged, not actually sent")
		notifier = notify.NewLogSender()
	}

	router := handlers.NewRouter(pool, redisClient, issuer, producer, notifier, cfg.RazorpayKeyID, cfg.RazorpayKeySecret, cfg.RazorpayWebhookSecret, cfg.VAPIDPublicKey, cfg.GroqAPIKey, cfg.GroqModel)
	router = appmiddleware.CORS(cfg.CORSOrigin)(router)

	go func() {
		log.Println("metrics listening on :9090")
		if err := http.ListenAndServe(":9090", metrics.Handler()); err != nil {
			log.Printf("metrics server stopped: %v", err)
		}
	}()

	log.Printf("FlashCart API listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
