package invoice

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSnapshotLinesAllocatesDiscountAndIncludedTax(t *testing.T) {
	source := []sourceItem{
		{name: "Headphones", quantity: 1, unitPrice: 10000, taxRate: 1800},
		{name: "Cable", quantity: 1, unitPrice: 5000, taxRate: 500},
	}
	items, tax := snapshotLines(source, 15000, 1500)
	if items[0].DiscountPaise != 1000 || items[1].DiscountPaise != 500 {
		t.Fatalf("discount not allocated proportionally: %+v", items)
	}
	if items[0].TaxPaise != 1373 || items[1].TaxPaise != 214 || tax != 1587 {
		t.Fatalf("unexpected included-tax amounts: items=%+v tax=%d", items, tax)
	}
	if items[0].LineTotalPaise+items[1].LineTotalPaise != 13500 {
		t.Fatalf("invoice line totals do not match charged total: %+v", items)
	}
}

func TestPDFContainsInvoiceAndPaymentDetails(t *testing.T) {
	content, err := PDF(Invoice{
		OrderID: "order-123", InvoiceNumber: "FC-202610-00000001",
		IssuedAt:     time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		CustomerName: "A Customer", CustomerEmail: "customer@example.com",
		SubtotalPaise: 10000, TotalPaise: 10000, TaxPaise: 1525,
		PaymentMethod: "RAZORPAY", PaymentReference: "pay_123",
		Items: []Item{{ProductName: "A product", Quantity: 1, UnitPricePaise: 10000, TaxRateBPS: 1800, TaxPaise: 1525, LineTotalPaise: 10000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF")) {
		t.Fatal("renderer did not produce a PDF document")
	}
	text := string(content)
	for _, value := range []string{"TAX INVOICE", "FC-202610-00000001", "customer@example.com", "pay_123", "TOTAL PAID"} {
		if !strings.Contains(text, value) {
			t.Errorf("PDF is missing %q", value)
		}
	}
}

func TestIncludedTaxHandlesZeroAndFullRate(t *testing.T) {
	if got := includedTax(10000, 0); got != 0 {
		t.Fatalf("zero tax rate produced %d", got)
	}
	if got := includedTax(10000, 10000); got != 5000 {
		t.Fatalf("100%% tax-inclusive price should contain 5000 paise tax, got %d", got)
	}
}
