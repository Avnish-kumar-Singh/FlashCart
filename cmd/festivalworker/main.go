package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/yourname/flashcart/internal/cache"
	"github.com/yourname/flashcart/internal/config"
	"github.com/yourname/flashcart/internal/db"
	"github.com/yourname/flashcart/internal/festival"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
)

func main() {
	cfg := config.Load()
	location, err := time.LoadLocation(cfg.FestivalTimezone)
	if err != nil {
		log.Fatalf("load festival timezone: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	redisClient, err := cache.NewClient(cfg.RedisAddr)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer redisClient.Close()
	producer := kafka.NewProducer(cfg.KafkaBrokers)
	defer producer.Close()

	var calendar festival.Provider
	if cfg.HolidayCalendarProvider == "nager" {
		baseURL := cfg.HolidayCalendarURL
		if baseURL == "" {
			baseURL = "https://date.nager.at/api/v3"
		}
		calendar = festival.NewCalendarClient(baseURL)
	} else {
		baseURL := cfg.HolidayCalendarURL
		if baseURL == "" {
			baseURL = "https://calendarific.com/api/v2"
		}
		calendar = festival.NewCalendarificClient(cfg.CalendarificAPIKey, baseURL)
	}
	worker := &festival.Worker{
		DB: pool, Redis: redisClient, Producer: producer,
		Calendar: calendar,
		Config: festival.WorkerConfig{
			CountryCode: cfg.HolidayCountryCode, Timezone: location,
			DiscountPercent:  cfg.FestivalDiscountPercent,
			StockPerProduct:  cfg.FestivalStockPerProduct,
			ProductsPerEvent: cfg.FestivalProductsPerEvent,
		},
	}
	go func() {
		log.Println("festival calendar worker started")
		worker.Run(ctx)
	}()
	go func() {
		if err := http.ListenAndServe(":9090", metrics.Handler()); err != nil {
			log.Printf("festival metrics server stopped: %v", err)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	log.Println("festival calendar worker metrics listening on :9091")
	if err := http.ListenAndServe(":9091", mux); err != nil {
		log.Printf("festival worker stopped: %v", err)
	}
}
