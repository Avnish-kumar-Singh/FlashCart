package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/flashcart/internal/auth"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/notify"
)

func NewRouter(pool *pgxpool.Pool, redisClient *redis.Client, issuer *auth.Issuer, producer *kafka.Producer, notifier notify.Sender, razorpayKeyID, razorpayKeySecret, razorpayWebhookSecret, vapidPublicKey, groqAPIKey, groqModel string) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(metrics.Middleware)

	if notifier == nil {
		notifier = notify.NewLogSender()
	}

	userH := &UserHandler{DB: pool, Issuer: issuer}
	pushH := &PushHandler{DB: pool, PublicKey: vapidPublicKey}
	aiH := &AIShoppingHandler{DB: pool, APIKey: groqAPIKey, Model: groqModel}
	productH := &ProductHandler{DB: pool}
	cartH := &CartHandler{Redis: redisClient}
	couponH := &CouponHandler{DB: pool, Redis: redisClient}
	orderH := &OrderHandler{DB: pool, Redis: redisClient, Producer: producer}
	invoiceH := &InvoiceHandler{DB: pool}
	flashSaleH := &FlashSaleHandler{DB: pool, Redis: redisClient, Producer: producer, Notifier: notifier}
	adminH := &AdminHandler{DB: pool}
	categoryH := &CategoryHandler{DB: pool}
	festivalH := &FestivalHandler{DB: pool}
	announcementH := &AnnouncementHandler{DB: pool}
	broadcastH := &BroadcastHandler{DB: pool, Notifier: notifier}
	wishlistH := &WishlistHandler{DB: pool}
	subscriptionH := &SubscriptionHandler{DB: pool}
	analyticsH := &AnalyticsHandler{DB: pool, Redis: redisClient}
	paymentH := NewPaymentHandler(pool, razorpayKeyID, razorpayKeySecret, razorpayWebhookSecret, producer)
	addressH := &AddressHandler{DB: pool}
	returnH := &ReturnHandler{DB: pool}
	supportH := &SupportHandler{DB: pool}

	requireAuth := appmiddleware.RequireAuth(issuer)
	requireAdmin := appmiddleware.RequireRole(issuer, "ADMIN")

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Get("/festivals/current", festivalH.Current)
	r.Get("/push/public-key", pushH.PublicVAPIDKey)

	r.Post("/payments/webhook", paymentH.Webhook)

	// Login is rate-limited per IP (not per user — the attacker doesn't have
	// a valid user yet) to slow down credential-stuffing / brute force.
	loginRateLimit := appmiddleware.RateLimit(redisClient, 10, time.Minute, "login", appmiddleware.ClientIP)
	aiRateLimit := appmiddleware.RateLimit(redisClient, 8, time.Minute, "ai-suggestions", appmiddleware.ClientIP)
	r.With(aiRateLimit).Post("/ai/suggestions", aiH.Suggest)

	r.Route("/users", func(r chi.Router) {
		r.Post("/register", userH.Register)
		r.With(loginRateLimit).Post("/login", userH.Login)
		r.Post("/refresh", userH.Refresh)
	})

	r.Route("/account", func(r chi.Router) {
		r.Use(requireAuth)
		r.Post("/push-subscriptions", pushH.Subscribe)
		r.Delete("/push-subscriptions", pushH.Unsubscribe)
		r.Get("/theme", userH.GetTheme)
		r.Put("/theme", userH.UpdateTheme)
		r.Get("/addresses", addressH.List)
		r.Post("/addresses", addressH.Create)
		r.Delete("/addresses/{id}", addressH.Delete)
	})

	r.Route("/products", func(r chi.Router) {
		r.Get("/", productH.List)
		r.With(requireAdmin).Post("/", productH.Create)
		r.Get("/{id}", productH.Get)
		r.With(requireAdmin).Put("/{id}", productH.Update)
		r.With(requireAdmin).Delete("/{id}", productH.Delete)
	})

	// Checkout is rate-limited per authenticated user: legitimate flash-sale
	// traffic still needs to get through, but a script hammering "buy" as
	// fast as possible for one account shouldn't get more attempts than a
	// normal user clicking the button.
	checkoutRateLimit := appmiddleware.RateLimit(redisClient, 5, time.Second, "checkout", func(r *http.Request) string {
		userID, _ := appmiddleware.UserIDFromContext(r.Context())
		return userID
	})

	r.Route("/cart", func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/", cartH.Get)
		r.Get("/coupons", couponH.List)
		r.Post("/coupons/validate", couponH.Validate)
		r.Post("/items", cartH.AddItem)
		r.Put("/items/{productID}", cartH.UpdateItem)
		r.Delete("/items/{productID}", cartH.RemoveItem)
		r.With(checkoutRateLimit).Post("/checkout", orderH.CreateFromCart)
	})

	r.Route("/flash-sales", func(r chi.Router) {
		r.With(requireAdmin).Post("/activate", flashSaleH.Activate)
		r.Get("/current", flashSaleH.Current)
		r.Get("/active", flashSaleH.Active)
		r.Get("/{saleID}", flashSaleH.Status)
		r.Post("/{saleID}/track", flashSaleH.Track)
		r.With(requireAuth).Post("/{saleID}/buy", flashSaleH.Buy)
		r.With(requireAdmin).Post("/{saleID}/deactivate", flashSaleH.Deactivate)
	})

	// Public: anyone can see the current banner or subscribe for alerts.
	// OptionalAuth recognizes a logged-in subscriber without forcing anonymous
	// visitors to sign in first.
	r.Get("/announcements/active", announcementH.Active)
	r.With(appmiddleware.OptionalAuth(issuer)).Post("/subscriptions", subscriptionH.Subscribe)

	r.Route("/payments", func(r chi.Router) {
		r.Use(requireAuth)
		r.Post("/razorpay/order", paymentH.CreateRazorpayOrder)
		r.Post("/razorpay/verify", paymentH.Verify)
		r.Get("/orders/{orderID}", paymentH.Status)
	})

	r.Route("/admin", func(r chi.Router) {
		r.Use(requireAdmin)
		r.Get("/analytics/coupons", couponH.Analytics)
		r.Get("/categories", categoryH.List)
		r.Post("/categories", categoryH.Create)
		r.Put("/categories/{id}", categoryH.Update)
		r.Delete("/categories/{id}", categoryH.Delete)
		r.Get("/stats", adminH.Stats)
		r.Get("/orders", adminH.Orders)
		r.Get("/payments", adminH.Payments)
		r.Post("/payments/{paymentID}/refund", paymentH.Refund)

		r.Get("/announcements", announcementH.List)
		r.Post("/announcements", announcementH.Create)
		r.Post("/announcements/{id}/deactivate", announcementH.Deactivate)

		r.Get("/broadcasts", broadcastH.List)
		r.Post("/broadcasts", broadcastH.Create)

		r.Get("/analytics/flash-sales", analyticsH.Overview)
		r.Get("/analytics/flash-sales/{saleID}", analyticsH.FlashSale)
	})

	r.Route("/wishlist", func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/", wishlistH.List)
		r.Post("/{productID}", wishlistH.Add)
		r.Delete("/{productID}", wishlistH.Remove)
	})

	r.Route("/orders", func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/", orderH.ListMine)
		r.Get("/{id}/invoice", invoiceH.Get)
		r.Get("/{id}/invoice.pdf", invoiceH.PDF)
		r.Get("/{id}", orderH.Get)
		r.Get("/{id}/tracking", orderH.GetTracking)
	})

	r.Route("/returns", func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/", returnH.List)
		r.Post("/", returnH.Create)
	})

	r.Route("/support", func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/tickets", supportH.List)
		r.Post("/tickets", supportH.Create)
	})

	return r
}
