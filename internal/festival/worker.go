package festival

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/inventory"
	"github.com/yourname/flashcart/internal/kafka"
)

type WorkerConfig struct {
	CountryCode      string
	Timezone         *time.Location
	DiscountPercent  int
	StockPerProduct  int
	ProductsPerEvent int
}

type Worker struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Producer *kafka.Producer
	Calendar Provider
	Config   WorkerConfig
	Now      func() time.Time
}

type festivalEvent struct {
	ID        string
	Name      string
	LocalName string
	StartsAt  time.Time
	EndsAt    time.Time
	Discount  int
}

type festivalSale struct {
	ID              string
	ProductID       string
	SaleID          string
	StockAllocated  int
	SalePricePaise  int64
	DiscountPercent int
	Remaining       int
	Status          string
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Worker) Run(ctx context.Context) {
	if err := validateWorkerConfig(w.Config); err != nil {
		log.Fatalf("festival worker configuration: %v", err)
	}
	var lastSync time.Time
	var nextSyncAttempt time.Time
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		now := w.now()
		due := (lastSync.IsZero() && (nextSyncAttempt.IsZero() || !now.Before(nextSyncAttempt))) || (!lastSync.IsZero() && now.Sub(lastSync) >= 24*time.Hour)
		if due {
			if err := w.SyncCalendar(ctx); err != nil {
				log.Printf("festival calendar sync failed; retrying in 10 minutes: %v", err)
				nextSyncAttempt = now.Add(10 * time.Minute)
			} else {
				lastSync = now
				nextSyncAttempt = time.Time{}
			}
		}
		if err := w.ProcessOnce(ctx); err != nil {
			log.Printf("festival scheduler tick failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) SyncCalendar(ctx context.Context) error {
	if w.Config.Timezone == nil {
		return errors.New("festival timezone is required")
	}
	now := w.now().In(w.Config.Timezone)
	for _, year := range []int{now.Year(), now.Year() + 1} {
		holidays, err := w.Calendar.Fetch(ctx, w.Config.CountryCode, year)
		if err != nil {
			return err
		}
		for _, holiday := range holidays {
			if !hasPublicType(holiday.Types) {
				continue
			}
			startsAt, endsAt, err := LocalWindow(holiday.Date, w.Config.Timezone)
			if err != nil {
				log.Printf("festival: skipping invalid date %q (%s): %v", holiday.Date, holiday.Name, err)
				continue
			}
			regions, err := json.Marshal(holiday.Counties)
			if err != nil {
				return err
			}
			if _, err := w.DB.Exec(ctx, `
				INSERT INTO festival_events (country_code,holiday_date,name,local_name,counties,global_holiday,starts_at,ends_at,discount_percent)
				VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9)
				ON CONFLICT(country_code,holiday_date,name) DO UPDATE SET
				local_name=EXCLUDED.local_name,counties=EXCLUDED.counties,global_holiday=EXCLUDED.global_holiday,
				starts_at=EXCLUDED.starts_at,ends_at=EXCLUDED.ends_at,updated_at=now()
				WHERE festival_events.status='SCHEDULED'`, strings.ToUpper(w.Config.CountryCode), holiday.Date,
				holiday.Name, holiday.LocalName, string(regions), holiday.Global, startsAt, endsAt, w.Config.DiscountPercent); err != nil {
				return fmt.Errorf("save holiday %s (%s): %w", holiday.Name, holiday.Date, err)
			}
		}
	}
	return nil
}

func hasPublicType(types []string) bool {
	for _, holidayType := range types {
		if strings.EqualFold(holidayType, "Public") {
			return true
		}
	}
	return false
}

func (w *Worker) ProcessOnce(ctx context.Context) error {
	if err := w.queueReminders(ctx); err != nil {
		return err
	}
	if err := w.activateDue(ctx); err != nil {
		return err
	}
	if err := w.endDue(ctx); err != nil {
		return err
	}
	return w.publishPendingStartNotices(ctx)
}

func (w *Worker) queueReminders(ctx context.Context) error {
	rows, err := w.DB.Query(ctx, `
		SELECT id::text,name,local_name,starts_at FROM festival_events
		WHERE status='SCHEDULED' AND reminder_queued_at IS NULL
		  AND starts_at > now() AND starts_at <= now() + interval '7 days'
		ORDER BY starts_at LIMIT 50`)
	if err != nil {
		return err
	}
	type reminder struct {
		id, name, localName string
		startsAt            time.Time
	}
	items := []reminder{}
	for rows.Next() {
		var item reminder
		if err := rows.Scan(&item.id, &item.name, &item.localName, &item.startsAt); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		tag, err := w.DB.Exec(ctx, `UPDATE festival_events SET reminder_queued_at=now(),updated_at=now() WHERE id=$1 AND reminder_queued_at IS NULL`, item.id)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		notice := events.FestivalNotice{FestivalID: item.id, Name: festivalDisplayName(item.localName, item.name), StartsAt: item.startsAt, Phase: "REMINDER"}
		if err := w.Producer.Publish(ctx, events.TopicFestivalNotice, item.id+":REMINDER", notice); err != nil {
			_, _ = w.DB.Exec(ctx, `UPDATE festival_events SET reminder_queued_at=NULL WHERE id=$1`, item.id)
			return err
		}
		_ = w.logActivity(ctx, item.id, "REMINDER_QUEUED", map[string]any{"starts_at": item.startsAt})
	}
	return nil
}

func (w *Worker) activateDue(ctx context.Context) error {
	rows, err := w.DB.Query(ctx, `
		SELECT id::text,name,local_name,starts_at,ends_at,discount_percent
		FROM festival_events
		WHERE starts_at<=now() AND ends_at>now() AND
		      (status='SCHEDULED' OR (status='ACTIVATING' AND worker_lease_until<now()))
		ORDER BY starts_at LIMIT 20`)
	if err != nil {
		return err
	}
	eventsToStart := []festivalEvent{}
	for rows.Next() {
		var item festivalEvent
		if err := rows.Scan(&item.ID, &item.Name, &item.LocalName, &item.StartsAt, &item.EndsAt, &item.Discount); err != nil {
			rows.Close()
			return err
		}
		eventsToStart = append(eventsToStart, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range eventsToStart {
		claimed, err := w.claimEvent(ctx, item.ID, "ACTIVATING")
		if err != nil || !claimed {
			continue
		}
		if err := w.startFestival(ctx, item); err != nil {
			_, _ = w.DB.Exec(ctx, `UPDATE festival_events SET worker_lease_until=now(),updated_at=now() WHERE id=$1`, item.ID)
			_ = w.logActivity(ctx, item.ID, "START_FAILED", map[string]any{"error": err.Error()})
			return err
		}
	}
	return nil
}

func (w *Worker) claimEvent(ctx context.Context, id, nextStatus string) (bool, error) {
	if nextStatus != "ACTIVATING" && nextStatus != "ENDING" {
		return false, errors.New("invalid festival worker state")
	}
	tag, err := w.DB.Exec(ctx, `
		UPDATE festival_events SET status=$2,worker_lease_until=now()+interval '2 minutes',updated_at=now()
		WHERE id=$1 AND ((status='SCHEDULED' AND $2='ACTIVATING') OR (status='ACTIVE' AND $2='ENDING') OR
		(status=$2 AND worker_lease_until<now()))`, id, nextStatus)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (w *Worker) startFestival(ctx context.Context, event festivalEvent) error {
	rows, err := w.DB.Query(ctx, `
		SELECT p.id::text FROM products p
		WHERE p.stock>0 AND NOT EXISTS(SELECT 1 FROM festival_sales fs WHERE fs.festival_event_id=$1 AND fs.product_id=p.id)
		ORDER BY p.stock DESC,p.created_at LIMIT $2`, event.ID, w.Config.ProductsPerEvent)
	if err != nil {
		return err
	}
	productIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		productIDs = append(productIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, productID := range productIDs {
		if err := w.prepareSale(ctx, event, productID); err != nil {
			return err
		}
	}
	var saleRows []festivalSale
	sales, err := w.DB.Query(ctx, `
		SELECT id::text,product_id::text,sale_id::text,stock_allocated,sale_price_paise,discount_percent,remaining_to_restore,status
		FROM festival_sales WHERE festival_event_id=$1 AND status IN ('ALLOCATED','ACTIVE')`, event.ID)
	if err != nil {
		return err
	}
	for sales.Next() {
		var sale festivalSale
		if err := sales.Scan(&sale.ID, &sale.ProductID, &sale.SaleID, &sale.StockAllocated, &sale.SalePricePaise, &sale.DiscountPercent, &sale.Remaining, &sale.Status); err != nil {
			sales.Close()
			return err
		}
		saleRows = append(saleRows, sale)
	}
	if err := sales.Err(); err != nil {
		sales.Close()
		return err
	}
	sales.Close()
	for _, sale := range saleRows {
		if sale.Status == "ACTIVE" {
			continue
		}
		if err := w.activateRedisSale(ctx, event, sale); err != nil {
			return err
		}
		if _, err := w.DB.Exec(ctx, `UPDATE festival_sales SET status='ACTIVE',updated_at=now() WHERE id=$1 AND status='ALLOCATED'`, sale.ID); err != nil {
			return err
		}
		_ = w.logActivity(ctx, event.ID, "PRODUCT_SALE_STARTED", map[string]any{"product_id": sale.ProductID, "sale_id": sale.SaleID, "discount_percent": sale.DiscountPercent})
	}
	if _, err := w.DB.Exec(ctx, `UPDATE festival_events SET status='ACTIVE',worker_lease_until=NULL,updated_at=now() WHERE id=$1 AND status='ACTIVATING'`, event.ID); err != nil {
		return err
	}
	return w.logActivity(ctx, event.ID, "FESTIVAL_SALE_STARTED", map[string]any{"name": festivalDisplayName(event.LocalName, event.Name)})
}

func (w *Worker) prepareSale(ctx context.Context, event festivalEvent, productID string) error {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var stock int
	var price int64
	if err := tx.QueryRow(ctx, `SELECT stock,price_paise FROM products WHERE id=$1 FOR UPDATE`, productID).Scan(&stock, &price); err != nil {
		return err
	}
	if stock <= 0 {
		return nil
	}
	allocated := stock
	if w.Config.StockPerProduct > 0 && allocated > w.Config.StockPerProduct {
		allocated = w.Config.StockPerProduct
	}
	salePrice := int64(math.Round(float64(price) * float64(100-w.Config.DiscountPercent) / 100))
	saleID := uuid.NewString()
	if tag, err := tx.Exec(ctx, `UPDATE products SET stock=stock-$1 WHERE id=$2 AND stock >= $1`, allocated, productID); err != nil {
		return err
	} else if tag.RowsAffected() != 1 {
		return errors.New("festival stock changed while preparing sale")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO festival_sales (festival_event_id,product_id,sale_id,stock_allocated,sale_price_paise,discount_percent,status)
		VALUES ($1,$2,$3,$4,$5,$6,'ALLOCATED') ON CONFLICT(festival_event_id,product_id) DO NOTHING`,
		event.ID, productID, saleID, allocated, salePrice, w.Config.DiscountPercent); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) activateRedisSale(ctx context.Context, event festivalEvent, sale festivalSale) error {
	var productName, image string
	if err := w.DB.QueryRow(ctx, `SELECT name,image_url FROM products WHERE id=$1`, sale.ProductID).Scan(&productName, &image); err != nil {
		return err
	}
	pipe := w.Redis.TxPipeline()
	pipe.Set(ctx, inventory.Key(sale.SaleID, "product_id"), sale.ProductID, 0)
	pipe.Set(ctx, inventory.Key(sale.SaleID, "stock"), sale.StockAllocated, 0)
	pipe.Set(ctx, inventory.Key(sale.SaleID, "price"), sale.SalePricePaise, 0)
	pipe.Set(ctx, inventory.Key(sale.SaleID, "discount_percent"), sale.DiscountPercent, 0)
	pipe.Set(ctx, inventory.Key(sale.SaleID, "start_ms"), event.StartsAt.UnixMilli(), 0)
	pipe.Set(ctx, inventory.Key(sale.SaleID, "end_ms"), event.EndsAt.UnixMilli(), 0)
	pipe.SAdd(ctx, inventory.ActiveSalesSetKey, sale.SaleID)
	_, err := pipe.Exec(ctx)
	return err
}

func (w *Worker) endDue(ctx context.Context) error {
	rows, err := w.DB.Query(ctx, `
		SELECT id::text,name,local_name,starts_at,ends_at,discount_percent
		FROM festival_events WHERE ends_at<=now() AND
		      (status='ACTIVE' OR (status='ENDING' AND worker_lease_until<now()))
		ORDER BY ends_at LIMIT 20`)
	if err != nil {
		return err
	}
	items := []festivalEvent{}
	for rows.Next() {
		var item festivalEvent
		if err := rows.Scan(&item.ID, &item.Name, &item.LocalName, &item.StartsAt, &item.EndsAt, &item.Discount); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		claimed, err := w.claimEvent(ctx, item.ID, "ENDING")
		if err != nil || !claimed {
			continue
		}
		if err := w.endFestival(ctx, item); err != nil {
			_, _ = w.DB.Exec(ctx, `UPDATE festival_events SET worker_lease_until=now() WHERE id=$1`, item.ID)
			return err
		}
	}
	return nil
}

func (w *Worker) endFestival(ctx context.Context, event festivalEvent) error {
	rows, err := w.DB.Query(ctx, `
		SELECT id::text,product_id::text,sale_id::text,stock_allocated,sale_price_paise,discount_percent,remaining_to_restore,status
		FROM festival_sales WHERE festival_event_id=$1 AND status IN ('ACTIVE','ENDING')`, event.ID)
	if err != nil {
		return err
	}
	sales := []festivalSale{}
	for rows.Next() {
		var sale festivalSale
		if err := rows.Scan(&sale.ID, &sale.ProductID, &sale.SaleID, &sale.StockAllocated, &sale.SalePricePaise, &sale.DiscountPercent, &sale.Remaining, &sale.Status); err != nil {
			rows.Close()
			return err
		}
		sales = append(sales, sale)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, sale := range sales {
		if sale.Status == "ACTIVE" {
			remaining, err := w.Redis.Get(ctx, inventory.Key(sale.SaleID, "stock")).Int()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			if _, err := w.DB.Exec(ctx, `UPDATE festival_sales SET status='ENDING',remaining_to_restore=$2,updated_at=now() WHERE id=$1 AND status='ACTIVE'`, sale.ID, remaining); err != nil {
				return err
			}
			sale.Remaining = remaining
		}
		pipe := w.Redis.TxPipeline()
		pipe.Set(ctx, inventory.Key(sale.SaleID, "stock"), 0, 0)
		pipe.Set(ctx, inventory.Key(sale.SaleID, "end_ms"), w.now().UnixMilli(), 0)
		pipe.SRem(ctx, inventory.ActiveSalesSetKey, sale.SaleID)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		tx, err := w.DB.Begin(ctx)
		if err != nil {
			return err
		}
		if sale.Remaining > 0 {
			if _, err := tx.Exec(ctx, `UPDATE products SET stock=stock+$1 WHERE id=$2`, sale.Remaining, sale.ProductID); err != nil {
				tx.Rollback(ctx)
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE festival_sales SET status='ENDED',updated_at=now() WHERE id=$1 AND status='ENDING'`, sale.ID); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		_ = w.logActivity(ctx, event.ID, "PRODUCT_SALE_ENDED", map[string]any{"product_id": sale.ProductID, "sale_id": sale.SaleID, "stock_restored": sale.Remaining})
	}
	if _, err := w.DB.Exec(ctx, `UPDATE festival_events SET status='ENDED',worker_lease_until=NULL,updated_at=now() WHERE id=$1 AND status='ENDING'`, event.ID); err != nil {
		return err
	}
	if err := w.logActivity(ctx, event.ID, "FESTIVAL_SALE_ENDED", map[string]any{"name": festivalDisplayName(event.LocalName, event.Name)}); err != nil {
		return err
	}
	return w.publishNotice(ctx, event, "ENDED")
}

func (w *Worker) publishPendingStartNotices(ctx context.Context) error {
	rows, err := w.DB.Query(ctx, `SELECT id::text,name,local_name,starts_at,ends_at,discount_percent FROM festival_events WHERE status='ACTIVE' AND start_notice_queued_at IS NULL ORDER BY starts_at LIMIT 50`)
	if err != nil {
		return err
	}
	items := []festivalEvent{}
	for rows.Next() {
		var item festivalEvent
		if err := rows.Scan(&item.ID, &item.Name, &item.LocalName, &item.StartsAt, &item.EndsAt, &item.Discount); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		if err := w.publishNotice(ctx, item, "STARTED"); err != nil {
			return err
		}
		if _, err := w.DB.Exec(ctx, `UPDATE festival_events SET start_notice_queued_at=now(),updated_at=now() WHERE id=$1 AND start_notice_queued_at IS NULL`, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) publishNotice(ctx context.Context, event festivalEvent, phase string) error {
	return w.Producer.Publish(ctx, events.TopicFestivalNotice, event.ID+":"+phase, events.FestivalNotice{
		FestivalID: event.ID, Name: festivalDisplayName(event.LocalName, event.Name), StartsAt: event.StartsAt,
		EndsAt: event.EndsAt, DiscountPercent: event.Discount, Phase: phase,
	})
}

func (w *Worker) logActivity(ctx context.Context, eventID, action string, details any) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = w.DB.Exec(ctx, `INSERT INTO festival_activity_log(festival_event_id,action,details) VALUES($1,$2,$3::jsonb)`, eventID, action, string(payload))
	return err
}

func festivalDisplayName(localName, name string) string {
	if strings.TrimSpace(localName) != "" {
		return localName
	}
	return name
}

func validateWorkerConfig(config WorkerConfig) error {
	if config.Timezone == nil {
		return errors.New("festival timezone is required")
	}
	if config.DiscountPercent < 1 || config.DiscountPercent > 50 {
		return errors.New("festival discount must be between 1 and 50 percent")
	}
	if config.StockPerProduct < 1 || config.ProductsPerEvent < 1 {
		return errors.New("festival stock and product limits must be positive")
	}
	return nil
}

var _ = validateWorkerConfig
