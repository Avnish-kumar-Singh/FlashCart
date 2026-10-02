package notify

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestInvoiceMessageIncludesPDFAttachment(t *testing.T) {
	pdf := []byte("%PDF-1.4 test invoice")
	from, _ := mail.ParseAddress("FlashCart <billing@example.com>")
	to, _ := mail.ParseAddress("Customer <customer@example.com>")
	message, err := invoiceMessage(from, to, "FC-202610-00000001", pdf)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("unexpected message content type %q: %v", mediaType, err)
	}
	parts := multipart.NewReader(parsed.Body, params["boundary"])
	var foundAttachment bool
	for {
		part, err := parts.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(part.Header.Get("Content-Disposition"), "attachment") {
			encoded, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(encoded), "\r\n", ""))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, pdf) {
				t.Fatalf("attachment differs from generated invoice PDF: %q", decoded)
			}
			foundAttachment = true
		}
	}
	if !foundAttachment {
		t.Fatal("invoice email has no PDF attachment")
	}
}
