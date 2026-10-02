package inventory

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const ReservationTTL = 5 * time.Minute

func Key(saleID, suffix string) string { return "flashsale:" + saleID + ":" + suffix }

// ActiveSalesSetKey holds every sale ID that has ever been activated and
// not yet explicitly deactivated. This is the fix for "only one flash sale
// shows up": the old design kept a single "current sale" pointer that each
// new Activate call overwrote, so a second product's sale silently orphaned
// the first. A set has no such ceiling — activating N products keeps all N
// visible, and the storefront lists every sale in the set whose window
// hasn't ended (see ListActive in the flashsale handler).
const ActiveSalesSetKey = "flashsale:active_sales"

func AddActiveSale(ctx context.Context, rdb *redis.Client, saleID string) error {
	return rdb.SAdd(ctx, ActiveSalesSetKey, saleID).Err()
}

func RemoveActiveSale(ctx context.Context, rdb *redis.Client, saleID string) error {
	return rdb.SRem(ctx, ActiveSalesSetKey, saleID).Err()
}

func ActiveSaleIDs(ctx context.Context, rdb *redis.Client) ([]string, error) {
	return rdb.SMembers(ctx, ActiveSalesSetKey).Result()
}

var reserveScript = redis.NewScript(`
local stock = tonumber(redis.call('GET', KEYS[1]) or '-1')
local start_ms = tonumber(redis.call('GET', KEYS[2]) or '0')
local end_ms = tonumber(redis.call('GET', KEYS[3]) or '0')
local now_ms = tonumber(ARGV[1])
local qty = tonumber(ARGV[2])
if now_ms < start_ms then return {0, 'NOT_STARTED'} end
if end_ms > 0 and now_ms >= end_ms then return {0, 'ENDED'} end
if qty < 1 then return {0, 'INVALID_QUANTITY'} end
if redis.call('EXISTS', KEYS[4]) == 1 then return {0, 'ALREADY_RESERVED'} end
if stock < qty then return {0, 'OUT_OF_STOCK'} end
redis.call('DECRBY', KEYS[1], qty)
redis.call('SET', KEYS[5], ARGV[3] .. '|' .. ARGV[2], 'EX', ARGV[4])
redis.call('SET', KEYS[4], ARGV[3], 'EX', ARGV[4])
return {1, 'RESERVED'}
`)

var releaseScript = redis.NewScript(`
local reservation = redis.call('GET', KEYS[2])
if not reservation then return {0, 'NOT_FOUND'} end
local qty = tonumber(string.match(reservation, '|(%d+)$') or '0')
if qty <= 0 then return {0, 'INVALID_RESERVATION'} end
redis.call('INCRBY', KEYS[1], qty)
redis.call('DEL', KEYS[2])
redis.call('DEL', KEYS[3])
return {1, 'RELEASED'}
`)

var consumeScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('DEL', KEYS[1])
redis.call('DEL', KEYS[2])
return 1
`)

func Reserve(ctx context.Context, rdb *redis.Client, saleID, userID, reservationID string, quantity int) (bool, string, error) {
	result, err := reserveScript.Run(ctx, rdb, []string{
		Key(saleID, "stock"), Key(saleID, "start_ms"), Key(saleID, "end_ms"),
		Key(saleID, "user:"+userID), Key(saleID, "reservation:"+reservationID),
	}, time.Now().UnixMilli(), quantity, reservationID, int(ReservationTTL/time.Second)).Result()
	if err != nil {
		return false, "", err
	}
	parts, ok := result.([]any)
	if !ok || len(parts) != 2 {
		return false, "INVALID_RESPONSE", nil
	}
	code, _ := parts[0].(int64)
	reason, _ := parts[1].(string)
	return code == 1, reason, nil
}

func Release(ctx context.Context, rdb *redis.Client, saleID, userID, reservationID string) error {
	_, err := releaseScript.Run(ctx, rdb, []string{
		Key(saleID, "stock"), Key(saleID, "reservation:"+reservationID), Key(saleID, "user:"+userID),
	}).Result()
	return err
}

func Consume(ctx context.Context, rdb *redis.Client, saleID, userID, reservationID string) error {
	_, err := consumeScript.Run(ctx, rdb, []string{
		Key(saleID, "reservation:"+reservationID), Key(saleID, "user:"+userID),
	}).Result()
	return err
}
