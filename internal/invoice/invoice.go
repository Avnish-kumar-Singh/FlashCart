package invoice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("order not found")
	ErrNotConfirmed = errors.New("order is not confirmed")
	ErrAmount       = errors.New("order total does not match its itemized amount")
)

type Item struct {
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPricePaise int64  `json:"unit_price_paise"`
	DiscountPaise  int64  `json:"discount_paise"`
	TaxRateBPS     int    `json:"tax_rate_bps"`
	TaxPaise       int64  `json:"tax_paise"`
	LineTotalPaise int64  `json:"line_total_paise"`
}

type Invoice struct {
	ID               string    `json:"id"`
	OrderID          string    `json:"order_id"`
	InvoiceNumber    string    `json:"invoice_number"`
	IssuedAt         time.Time `json:"issued_at"`
	CustomerName     string    `json:"customer_name"`
	CustomerEmail    string    `json:"customer_email"`
	SubtotalPaise    int64     `json:"subtotal_paise"`
	DiscountPaise    int64     `json:"discount_paise"`
	TaxPaise         int64     `json:"tax_paise"`
	TotalPaise       int64     `json:"total_paise"`
	PaymentMethod    string    `json:"payment_method"`
	PaymentReference string    `json:"payment_reference"`
	EmailStatus      string    `json:"email_status"`
	Items            []Item    `json:"items"`
}

type sourceItem struct {
	name      string
	quantity  int
	unitPrice int64
	taxRate   int
}

func Generate(ctx context.Context, db *pgxpool.Pool, orderID, expectedUserID string) (Invoice, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return Invoice{}, err
	}
	defer tx.Rollback(ctx)

	var status, paymentMethod, ownerID, customerName, customerEmail, paymentReference string
	var totalPaise, discountPaise int64
	var emailEligible bool
	err = tx.QueryRow(ctx, `
		SELECT o.status, o.payment_method, o.user_id::text, u.name, u.email,
		       o.total_paise, o.discount_paise, (o.invoice_eligible_at IS NOT NULL),
	       COALESCE(p.razorpay_payment_id, '')
		FROM orders o
		JOIN users u ON u.id=o.user_id
		LEFT JOIN payments p ON p.order_id=o.id
		WHERE o.id=$1
		FOR UPDATE OF o`, orderID).Scan(&status, &paymentMethod, &ownerID, &customerName,
		&customerEmail, &totalPaise, &discountPaise, &emailEligible, &paymentReference)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && ownerID != expectedUserID) {
		return Invoice{}, ErrNotFound
	}
	if err != nil {
		return Invoice{}, err
	}
	if status != "CONFIRMED" {
		return Invoice{}, ErrNotConfirmed
	}
	if paymentReference == "" {
		paymentReference = paymentMethod
	}

	var existingID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM invoices WHERE order_id=$1`, orderID).Scan(&existingID)
	if err == nil {
		if emailEligible {
			if err := queueEmail(ctx, tx, existingID); err != nil {
				return Invoice{}, err
			}
		}
		result, loadErr := load(ctx, tx, existingID)
		if loadErr != nil {
			return Invoice{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return Invoice{}, err
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT product_name_snapshot, quantity, unit_price_paise, tax_rate_bps
		FROM order_items WHERE order_id=$1 ORDER BY id`, orderID)
	if err != nil {
		return Invoice{}, err
	}
	sourceItems := []sourceItem{}
	var subtotal int64
	for rows.Next() {
		var item sourceItem
		if err := rows.Scan(&item.name, &item.quantity, &item.unitPrice, &item.taxRate); err != nil {
			rows.Close()
			return Invoice{}, err
		}
		if item.name == "" || item.quantity <= 0 || item.unitPrice < 0 || item.taxRate < 0 || item.taxRate > 10000 {
			rows.Close()
			return Invoice{}, errors.New("invalid order item snapshot")
		}
		lineAmount := item.unitPrice * int64(item.quantity)
		if item.quantity != 0 && lineAmount/int64(item.quantity) != item.unitPrice {
			rows.Close()
			return Invoice{}, ErrAmount
		}
		subtotal += lineAmount
		if subtotal < lineAmount {
			rows.Close()
			return Invoice{}, ErrAmount
		}
		sourceItems = append(sourceItems, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Invoice{}, err
	}
	rows.Close()
	if len(sourceItems) == 0 || discountPaise < 0 || discountPaise > subtotal || subtotal-discountPaise != totalPaise {
		return Invoice{}, ErrAmount
	}

	items, taxTotal := snapshotLines(sourceItems, subtotal, discountPaise)
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT nextval('invoice_number_seq')`).Scan(&seq); err != nil {
		return Invoice{}, err
	}
	issuedAt := time.Now().UTC()
	invoiceNumber := fmt.Sprintf("FC-%s-%08d", issuedAt.Format("200601"), seq)
	var invoiceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO invoices (order_id, invoice_number, issued_at, customer_name, customer_email,
		                      subtotal_paise, discount_paise, tax_paise, total_paise,
		                      payment_method, payment_reference)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id::text`, orderID, invoiceNumber, issuedAt, customerName, customerEmail,
		subtotal, discountPaise, taxTotal, totalPaise, paymentMethod, paymentReference).Scan(&invoiceID)
	if err != nil {
		return Invoice{}, err
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoice_items (invoice_id, product_name, quantity, unit_price_paise,
			                          discount_paise, tax_rate_bps, tax_paise, line_total_paise)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, invoiceID, item.ProductName, item.Quantity,
			item.UnitPricePaise, item.DiscountPaise, item.TaxRateBPS, item.TaxPaise, item.LineTotalPaise); err != nil {
			return Invoice{}, err
		}
	}
	if emailEligible {
		if err := queueEmail(ctx, tx, invoiceID); err != nil {
			return Invoice{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Invoice{}, err
	}

	return Invoice{
		ID: invoiceID, OrderID: orderID, InvoiceNumber: invoiceNumber, IssuedAt: issuedAt,
		CustomerName: customerName, CustomerEmail: customerEmail, SubtotalPaise: subtotal,
		DiscountPaise: discountPaise, TaxPaise: taxTotal, TotalPaise: totalPaise,
		PaymentMethod: paymentMethod, PaymentReference: paymentReference, EmailStatus: "PENDING", Items: items,
	}, nil
}

func ClaimEmail(ctx context.Context, db *pgxpool.Pool, invoiceID string) (bool, error) {
	_, claimed, err := claimEmail(ctx, db, invoiceID)
	return claimed, err
}

func ClaimNextEmail(ctx context.Context, db *pgxpool.Pool) (string, bool, error) {
	return claimEmail(ctx, db, "")
}

func claimEmail(ctx context.Context, db *pgxpool.Pool, invoiceID string) (string, bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)
	var claimedID string
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM invoice_email_outbox
			WHERE ($1 = '' OR invoice_id::text=$1)
			  AND ((status='PENDING' AND next_attempt_at <= now()) OR
			       (status='PROCESSING' AND claimed_at < now() - interval '5 minutes'))
			ORDER BY next_attempt_at, created_at
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE invoice_email_outbox job
		SET status='PROCESSING', attempts=job.attempts+1, claimed_at=now(), updated_at=now()
		FROM candidate WHERE job.id=candidate.id
		RETURNING job.invoice_id::text`, invoiceID).Scan(&claimedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE invoices SET email_status='SENDING', email_attempted_at=now() WHERE id=$1`, claimedID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return claimedID, true, nil
}

func LoadByID(ctx context.Context, db *pgxpool.Pool, invoiceID string) (Invoice, error) {
	return load(ctx, db, invoiceID)
}

func MarkEmailSent(ctx context.Context, db *pgxpool.Pool, invoiceID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE invoices SET email_status='SENT', email_sent_at=now() WHERE id=$1`, invoiceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE invoice_email_outbox SET status='SENT', sent_at=now(), claimed_at=NULL, last_error='', updated_at=now() WHERE invoice_id=$1`, invoiceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func MarkEmailFailed(ctx context.Context, db *pgxpool.Pool, invoiceID string) error {
	return MarkEmailRetry(ctx, db, invoiceID, "email delivery failed")
}

func MarkEmailRetry(ctx context.Context, db *pgxpool.Pool, invoiceID, reason string) error {
	if len(reason) > 1000 {
		reason = reason[:1000]
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE invoices SET email_status='FAILED' WHERE id=$1`, invoiceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE invoice_email_outbox
		SET status='PENDING', claimed_at=NULL, last_error=$2,
		    next_attempt_at=now() + LEAST(3600::double precision, 30 * power(2, LEAST(attempts - 1, 7))) * interval '1 second',
		    updated_at=now()
		WHERE invoice_id=$1`, invoiceID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func queueEmail(ctx context.Context, tx pgx.Tx, invoiceID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO invoice_email_outbox(invoice_id) VALUES($1) ON CONFLICT(invoice_id) DO NOTHING`, invoiceID)
	return err
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func load(ctx context.Context, q querier, invoiceID string) (Invoice, error) {
	var result Invoice
	err := q.QueryRow(ctx, `
		SELECT id::text, order_id::text, invoice_number, issued_at, customer_name, customer_email,
		       subtotal_paise, discount_paise, tax_paise, total_paise, payment_method,
		       payment_reference, email_status
		FROM invoices WHERE id=$1`, invoiceID).Scan(&result.ID, &result.OrderID, &result.InvoiceNumber,
		&result.IssuedAt, &result.CustomerName, &result.CustomerEmail, &result.SubtotalPaise,
		&result.DiscountPaise, &result.TaxPaise, &result.TotalPaise, &result.PaymentMethod,
		&result.PaymentReference, &result.EmailStatus)
	if err != nil {
		return Invoice{}, err
	}
	result.Items = []Item{}
	rows, err := q.Query(ctx, `
		SELECT product_name, quantity, unit_price_paise, discount_paise,
		       tax_rate_bps, tax_paise, line_total_paise
		FROM invoice_items WHERE invoice_id=$1 ORDER BY id`, invoiceID)
	if err != nil {
		return Invoice{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductName, &item.Quantity, &item.UnitPricePaise,
			&item.DiscountPaise, &item.TaxRateBPS, &item.TaxPaise, &item.LineTotalPaise); err != nil {
			return Invoice{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func snapshotLines(source []sourceItem, subtotal, discount int64) ([]Item, int64) {
	items := make([]Item, len(source))
	var cumulative int64
	for i, line := range source {
		gross := line.unitPrice * int64(line.quantity)
		before := proportionalFloor(cumulative, subtotal, discount)
		cumulative += gross
		after := proportionalFloor(cumulative, subtotal, discount)
		lineDiscount := after - before
		lineTotal := gross - lineDiscount
		tax := includedTax(lineTotal, line.taxRate)
		items[i] = Item{ProductName: line.name, Quantity: line.quantity, UnitPricePaise: line.unitPrice,
			DiscountPaise: lineDiscount, TaxRateBPS: line.taxRate, TaxPaise: tax, LineTotalPaise: lineTotal}
	}
	return items, sumTax(items)
}

func proportionalFloor(part, total, amount int64) int64 {
	if part <= 0 || total <= 0 || amount <= 0 {
		return 0
	}
	value := new(big.Int).Mul(big.NewInt(part), big.NewInt(amount))
	value.Quo(value, big.NewInt(total))
	return value.Int64()
}

func includedTax(gross int64, rateBPS int) int64 {
	if gross <= 0 || rateBPS <= 0 {
		return 0
	}
	denominator := int64(10000 + rateBPS)
	quotient, remainder := gross/denominator, gross%denominator
	return quotient*int64(rateBPS) + (remainder*int64(rateBPS)+denominator/2)/denominator
}

func sumTax(items []Item) int64 {
	var total int64
	for _, item := range items {
		total += item.TaxPaise
	}
	return total
}

func PDF(doc Invoice) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetMargins(10, 12, 10)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 20)
	pdf.SetTextColor(25, 55, 91)
	pdf.CellFormat(110, 11, "FLASHCART", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "B", 15)
	pdf.SetTextColor(25, 35, 48)
	pdf.CellFormat(80, 11, "TAX INVOICE", "", 1, "R", false, 0, "")
	pdf.SetDrawColor(210, 218, 228)
	pdf.Line(10, 27, 200, 27)

	pdf.SetY(32)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(95, 6, "Invoice: "+doc.InvoiceNumber, "", 0, "L", false, 0, "")
	pdf.CellFormat(95, 6, "Issued: "+doc.IssuedAt.UTC().Format("02 Jan 2006 15:04 UTC"), "", 1, "R", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(95, 6, "Order: "+doc.OrderID, "", 0, "L", false, 0, "")
	pdf.CellFormat(95, 6, "Payment: "+doc.PaymentMethod, "", 1, "R", false, 0, "")
	if doc.PaymentReference != "" {
		pdf.CellFormat(190, 6, "Payment reference: "+doc.PaymentReference, "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(190, 6, "BILLED TO", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(190, 6, doc.CustomerName, "", 1, "L", false, 0, "")
	pdf.CellFormat(190, 6, doc.CustomerEmail, "", 1, "L", false, 0, "")
	pdf.Ln(6)

	widths := []float64{72, 13, 25, 24, 24, 32}
	headers := []string{"Item", "Qty", "Unit price", "Discount", "Tax incl.", "Line total"}
	pdf.SetFillColor(238, 243, 249)
	pdf.SetFont("Arial", "B", 8)
	for i, header := range headers {
		pdf.CellFormat(widths[i], 8, header, "B", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Arial", "", 8)
	for _, item := range doc.Items {
		nameLines := pdf.SplitText(item.ProductName, widths[0]-3)
		rowHeight := float64(len(nameLines)) * 5
		if rowHeight < 8 {
			rowHeight = 8
		}
		if pdf.GetY()+rowHeight > 275 {
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 8)
			for i, header := range headers {
				pdf.CellFormat(widths[i], 8, header, "B", 0, "L", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Arial", "", 8)
		}
		y := pdf.GetY()
		x := 10.0
		pdf.MultiCell(widths[0], 5, item.ProductName, "", "L", false)
		pdf.SetXY(x+widths[0], y)
		pdf.CellFormat(widths[1], rowHeight, fmt.Sprintf("%d", item.Quantity), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[2], rowHeight, formatMoney(item.UnitPricePaise), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], rowHeight, formatMoney(item.DiscountPaise), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], rowHeight, formatMoney(item.TaxPaise), "", 0, "R", false, 0, "")
		pdf.CellFormat(widths[5], rowHeight, formatMoney(item.LineTotalPaise), "", 1, "R", false, 0, "")
		if rowHeight > float64(len(nameLines))*5 {
			pdf.SetY(y + rowHeight)
		}
		pdf.SetDrawColor(230, 234, 240)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	}

	pdf.Ln(5)
	pdf.SetFont("Arial", "", 10)
	writeSummary := func(label string, amount int64, bold bool) {
		style := ""
		if bold {
			style = "B"
		}
		pdf.SetFont("Arial", style, 10)
		pdf.CellFormat(135, 7, label, "", 0, "R", false, 0, "")
		pdf.CellFormat(55, 7, formatMoney(amount), "", 1, "R", false, 0, "")
	}
	writeSummary("Items subtotal (tax included)", doc.SubtotalPaise, false)
	writeSummary("Discount", -doc.DiscountPaise, false)
	writeSummary("GST included in total", doc.TaxPaise, false)
	pdf.SetDrawColor(25, 55, 91)
	pdf.Line(145, pdf.GetY()+1, 200, pdf.GetY()+1)
	writeSummary("TOTAL PAID", doc.TotalPaise, true)
	pdf.SetY(-18)
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(100, 110, 125)
	pdf.CellFormat(190, 5, "Thank you for shopping with FlashCart.", "", 1, "C", false, 0, "")

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func formatMoney(paise int64) string {
	if paise < 0 {
		return "-INR " + formatMoney(-paise)[4:]
	}
	whole, fraction := paise/100, paise%100
	wholeText := fmt.Sprintf("%d", whole)
	for i := len(wholeText) - 3; i > 0; i -= 3 {
		wholeText = wholeText[:i] + "," + wholeText[i:]
	}
	return fmt.Sprintf("INR %s.%02d", wholeText, fraction)
}
