// Command notificationworker consumes order events and dispatches durable
// invoice email jobs through SMTP while logging non-invoice notifications:
//
//	Order Service -> Kafka -> Notification Service -> Email / SMS / Push
//
// Invoice jobs are stored with the invoice in PostgreSQL and retried
// independently of Kafka delivery so an email outage cannot lose an order.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/yourname/flashcart/internal/config"
	"github.com/yourname/flashcart/internal/db"
	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/invoice"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
	notifications "github.com/yourname/flashcart/internal/notify"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	mailer := notifications.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
	if mailer.Configured() {
		log.Printf("invoice email enabled via SMTP host %s", cfg.SMTPHost)
	} else {
		log.Printf("SMTP_HOST and SMTP_FROM are not configured; invoices will be generated but email delivery remains pending")
	}
	go retryPendingInvoiceEmails(ctx, pool, mailer)
	var smsSender notifications.Sender
	if cfg.TwilioAccountSID != "" && cfg.TwilioAuthToken != "" && (cfg.TwilioFromNumber != "" || cfg.TwilioMessagingServiceSID != "") {
		smsSender = notifications.NewTwilioSender(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioFromNumber, cfg.TwilioDefaultCountryCode, cfg.TwilioMessagingServiceSID, notifications.NewLogSender())
	} else {
		log.Printf("festival SMS notices disabled: Twilio credentials are not configured")
	}

	confirmedConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicOrderConfirmed, "notification-worker")
	defer confirmedConsumer.Close()

	cancelledConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicOrderCancelled, "notification-worker")
	defer cancelledConsumer.Close()
	festivalConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicFestivalNotice, "festival-notification-worker")
	defer festivalConsumer.Close()

	go func() {
		if err := http.ListenAndServe(":9090", metrics.Handler()); err != nil {
			log.Printf("metrics server stopped: %v", err)
		}
	}()

	log.Println("notification worker started, waiting for order.confirmed / order.cancelled events")

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		confirmedConsumer.Run(ctx, func(ctx context.Context, key, value []byte) error {
			var evt events.OrderConfirmed
			if err := json.Unmarshal(value, &evt); err != nil {
				log.Printf("notification worker: bad order.confirmed message, dropping: %v", err)
				return nil
			}
			if err := processConfirmed(ctx, pool, mailer, evt); err != nil {
				return err
			}
			notify(evt.UserID, "ORDER_CONFIRMED", "Your order "+evt.OrderID+" is confirmed!")
			metrics.WorkerMessages.WithLabelValues("notification", "success").Inc()
			return nil
		})
	}()

	go func() {
		defer wg.Done()
		festivalConsumer.Run(ctx, func(ctx context.Context, key, value []byte) error {
			var evt events.FestivalNotice
			if err := json.Unmarshal(value, &evt); err != nil {
				log.Printf("festival notification worker: bad message, dropping: %v", err)
				return nil
			}
			if err := sendFestivalNotice(ctx, pool, mailer, smsSender, cfg, evt); err != nil {
				metrics.WorkerMessages.WithLabelValues("festival_notification", "error").Inc()
				return err
			}
			metrics.WorkerMessages.WithLabelValues("festival_notification", "success").Inc()
			return nil
		})
	}()

	go func() {
		defer wg.Done()
		cancelledConsumer.Run(ctx, func(ctx context.Context, key, value []byte) error {
			var evt events.OrderCancelled
			if err := json.Unmarshal(value, &evt); err != nil {
				log.Printf("notification worker: bad order.cancelled message, dropping: %v", err)
				return nil
			}
			notify(evt.UserID, "ORDER_CANCELLED", "Your order "+evt.OrderID+" was cancelled: "+evt.Reason)
			metrics.WorkerMessages.WithLabelValues("notification", "success").Inc()
			return nil
		})
	}()

	wg.Wait()
}

func sendFestivalNotice(ctx context.Context, pool *pgxpool.Pool, mailer *notifications.SMTPMailer, sms notifications.Sender, cfg config.Config, evt events.FestivalNotice) error {
	title := evt.Name + " festival sale"
	body := "Festival offers are live now: " + evt.Name + ". Shop FlashCart: /flash-sale"
	if evt.Phase == "REMINDER" {
		body = evt.Name + " is coming soon. Festival offers start " + evt.StartsAt.Format("02 Jan 2006") + ". Shop FlashCart: /flash-sale"
	}
	if evt.Phase == "ENDED" {
		body = "The " + evt.Name + " festival sale has ended. See you at the next FlashCart festival offer."
	}
	rows, err := pool.Query(ctx, `SELECT id::text,email,phone FROM users ORDER BY id`)
	if err != nil {
		return err
	}
	type recipient struct{ userID, email, phone string }
	recipients := []recipient{}
	for rows.Next() {
		var item recipient
		if err := rows.Scan(&item.userID, &item.email, &item.phone); err != nil {
			rows.Close()
			return err
		}
		recipients = append(recipients, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, user := range recipients {
		if mailer.Configured() && user.email != "" {
			if err := mailer.SendText(ctx, user.email, title, body); err != nil {
				return fmt.Errorf("email festival notice to user %s: %w", user.userID, err)
			}
		}
		if sms != nil && user.phone != "" {
			if err := sms.Send(ctx, notifications.ChannelSMS, notifications.Recipient{Email: user.email, Phone: user.phone}, body); err != nil {
				return fmt.Errorf("SMS festival notice to user %s: %w", user.userID, err)
			}
		}
		if cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "" {
			if err := sendFestivalPush(ctx, pool, cfg, user.userID, title, body); err != nil {
				return err
			}
		}
	}
	log.Printf("festival notice processed phase=%s event=%s recipients=%d email=%t sms=%t push=%t", evt.Phase, evt.FestivalID, len(recipients), mailer.Configured(), sms != nil, cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "")
	return nil
}

func sendFestivalPush(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, userID, title, body string) error {
	rows, err := pool.Query(ctx, `SELECT id::text,endpoint,p256dh,auth_secret FROM web_push_subscriptions WHERE user_id=$1`, userID)
	if err != nil {
		return err
	}
	type pushSubscription struct{ id, endpoint, key, auth string }
	subscriptions := []pushSubscription{}
	for rows.Next() {
		var item pushSubscription
		if err := rows.Scan(&item.id, &item.endpoint, &item.key, &item.auth); err != nil {
			rows.Close()
			return err
		}
		subscriptions = append(subscriptions, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	payload, err := json.Marshal(map[string]string{"title": title, "body": body, "url": "/flash-sale", "tag": "festival-" + title})
	if err != nil {
		return err
	}
	for _, item := range subscriptions {
		response, err := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: item.endpoint,
			Keys:     webpush.Keys{P256dh: item.key, Auth: item.auth},
		}, &webpush.Options{Subscriber: cfg.VAPIDSubject, VAPIDPublicKey: cfg.VAPIDPublicKey, VAPIDPrivateKey: cfg.VAPIDPrivateKey, TTL: 3600})
		if err != nil {
			return fmt.Errorf("web push for user %s: %w", userID, err)
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusGone || response.StatusCode == http.StatusNotFound {
			_, _ = pool.Exec(ctx, `DELETE FROM web_push_subscriptions WHERE id=$1`, item.id)
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("web push provider returned HTTP %d", response.StatusCode)
		}
	}
	return nil
}

func processConfirmed(ctx context.Context, pool *pgxpool.Pool, mailer *notifications.SMTPMailer, evt events.OrderConfirmed) error {
	doc, err := invoice.Generate(ctx, pool, evt.OrderID, evt.UserID)
	if err != nil {
		return err
	}
	if !mailer.Configured() {
		log.Printf("invoice %s created and queued; email pending SMTP configuration", doc.InvoiceNumber)
		return nil
	}
	return dispatchPendingEmails(ctx, pool, mailer)
}

func retryPendingInvoiceEmails(ctx context.Context, pool *pgxpool.Pool, mailer *notifications.SMTPMailer) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	processPendingInvoices(ctx, pool, mailer)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processPendingInvoices(ctx, pool, mailer)
		}
	}
}

func processPendingInvoices(ctx context.Context, pool *pgxpool.Pool, mailer *notifications.SMTPMailer) {
	rows, err := pool.Query(ctx, `
		SELECT o.id::text, o.user_id::text
		FROM orders o LEFT JOIN invoices i ON i.order_id=o.id
		WHERE o.status='CONFIRMED' AND o.invoice_eligible_at IS NOT NULL AND i.id IS NULL
		ORDER BY o.created_at LIMIT 100`)
	if err != nil {
		log.Printf("invoice recovery scan failed: %v", err)
		return
	}
	type pendingInvoice struct{ orderID, userID string }
	pending := []pendingInvoice{}
	for rows.Next() {
		var item pendingInvoice
		if err := rows.Scan(&item.orderID, &item.userID); err != nil {
			log.Printf("invoice recovery scan row failed: %v", err)
			continue
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("invoice recovery scan failed: %v", err)
	}
	rows.Close()
	for _, item := range pending {
		if _, err := invoice.Generate(ctx, pool, item.orderID, item.userID); err != nil {
			log.Printf("invoice generation retry failed for order %s: %v", item.orderID, err)
		}
	}
	if mailer.Configured() {
		if err := dispatchPendingEmails(ctx, pool, mailer); err != nil {
			log.Printf("invoice email outbox processing failed: %v", err)
		}
	}
}

func dispatchPendingEmails(ctx context.Context, pool *pgxpool.Pool, mailer *notifications.SMTPMailer) error {
	if !mailer.Configured() {
		return nil
	}
	for range 20 {
		invoiceID, claimed, err := invoice.ClaimNextEmail(ctx, pool)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		doc, err := invoice.LoadByID(ctx, pool, invoiceID)
		if err == nil {
			var pdf []byte
			pdf, err = invoice.PDF(doc)
			if err == nil {
				err = mailer.SendInvoice(ctx, doc.CustomerEmail, doc.InvoiceNumber, pdf)
			}
		}
		if err != nil {
			if markErr := invoice.MarkEmailRetry(ctx, pool, invoiceID, err.Error()); markErr != nil {
				return markErr
			}
			log.Printf("invoice email attempt failed for invoice %s: %v", invoiceID, err)
			continue
		}
		if err := invoice.MarkEmailSent(ctx, pool, invoiceID); err != nil {
			return err
		}
		log.Printf("invoice %s emailed to registered account address", doc.InvoiceNumber)
	}
	return nil
}

// notify stands in for Email + SMS + Push fan-out. Each channel would
// normally be its own goroutine/call with its own retry policy; logging
// once per channel here keeps the simulation obviously not idempotent-
// sensitive (unlike payment/inventory, sending a notification twice is a
// UX annoyance, not a correctness bug).
func notify(userID, eventType, message string) {
	for _, channel := range []string{"EMAIL", "SMS", "PUSH"} {
		log.Printf("[%s] -> user=%s event=%s message=%q", channel, userID, eventType, message)
	}
}
