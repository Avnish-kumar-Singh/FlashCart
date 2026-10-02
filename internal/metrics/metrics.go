package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flashcart", Name: "http_requests_total", Help: "Total HTTP requests.",
	}, []string{"method", "route", "status"})
	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "flashcart", Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
	FlashSaleReservations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flashcart", Name: "flash_sale_reservations_total", Help: "Flash-sale reservation outcomes.",
	}, []string{"result"})
	WorkerMessages = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flashcart", Name: "worker_messages_total", Help: "Messages handled by workers.",
	}, []string{"worker", "result"})
	PaymentAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flashcart", Name: "payment_attempts_total", Help: "Payment attempts by outcome.",
	}, []string{"result"})
	NotificationsSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "flashcart", Name: "notifications_sent_total", Help: "Simulated email/WhatsApp notifications sent, by reason.",
	}, []string{"reason"})
)

func init() {
	prometheus.MustRegister(HTTPRequests, HTTPRequestDuration, FlashSaleReservations, WorkerMessages, PaymentAttempts, NotificationsSent)
}

func Handler() http.Handler { return promhttp.Handler() }

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		route := r.URL.Path
		if pattern := routePattern(r); pattern != "" {
			route = pattern
		}
		HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rw.status)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(b []byte) (int, error) { return w.ResponseWriter.Write(b) }

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		return rc.RoutePattern()
	}
	return ""
}
