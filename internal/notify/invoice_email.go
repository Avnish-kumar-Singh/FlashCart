package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

type InvoiceMailer interface {
	SendInvoice(ctx context.Context, recipient, invoiceNumber string, pdf []byte) error
}

type SMTPMailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (m *SMTPMailer) SendText(ctx context.Context, recipient, subject, body string) error {
	if !m.Configured() {
		return errors.New("SMTP host and sender address are required")
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("invalid SMTP sender address: %w", err)
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil {
		return fmt.Errorf("invalid notification recipient address: %w", err)
	}
	message, err := textEmailMessage(from, to, subject, body)
	if err != nil {
		return err
	}
	return m.deliver(ctx, from.Address, to.Address, message)
}

func NewSMTPMailer(host, port, username, password, from string) *SMTPMailer {
	return &SMTPMailer{Host: host, Port: port, Username: username, Password: password, From: from}
}

func (m *SMTPMailer) Configured() bool {
	return strings.TrimSpace(m.Host) != "" && strings.TrimSpace(m.From) != ""
}

func (m *SMTPMailer) SendInvoice(ctx context.Context, recipient, invoiceNumber string, pdf []byte) error {
	if !m.Configured() {
		return errors.New("SMTP host and sender address are required")
	}
	if len(pdf) == 0 {
		return errors.New("invoice PDF is empty")
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("invalid SMTP sender address: %w", err)
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil {
		return fmt.Errorf("invalid invoice recipient address: %w", err)
	}
	message, err := invoiceMessage(from, to, invoiceNumber, pdf)
	if err != nil {
		return err
	}
	return m.deliver(ctx, from.Address, to.Address, message)
}

func (m *SMTPMailer) deliver(ctx context.Context, sender, recipient string, message []byte) error {
	port := m.Port
	if port == "" {
		port = "587"
	}
	address := net.JoinHostPort(m.Host, port)
	conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return fmt.Errorf("start SMTP session: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support required STARTTLS encryption")
	}
	if err := client.StartTLS(&tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("start SMTP TLS: %w", err)
	}
	if m.Username != "" || m.Password != "" {
		if err := client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return fmt.Errorf("authenticate with SMTP server: %w", err)
		}
	}
	if err := client.Mail(sender); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP message: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("send SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	return client.Quit()
}

func textEmailMessage(from, to *mail.Address, subject, bodyText string) ([]byte, error) {
	header := make(textproto.MIMEHeader)
	header.Set("From", from.String())
	header.Set("To", to.String())
	header.Set("Subject", mime.QEncoding.Encode("UTF-8", subject))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", "text/plain; charset=utf-8")
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	var message bytes.Buffer
	if _, err := io.WriteString(&message, headerToString(header)+"\r\n"); err != nil {
		return nil, err
	}
	writer := quotedprintable.NewWriter(&message)
	if _, err := io.WriteString(writer, bodyText+"\r\n"); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return message.Bytes(), nil
}

func invoiceMessage(from, to *mail.Address, invoiceNumber string, pdf []byte) ([]byte, error) {
	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	filename := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return -1
	}, invoiceNumber)
	if filename == "" {
		filename = "invoice"
	}

	header := make(textproto.MIMEHeader)
	header.Set("From", from.String())
	header.Set("To", to.String())
	header.Set("Subject", mime.QEncoding.Encode("UTF-8", "FlashCart invoice "+invoiceNumber))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", "multipart/mixed; boundary="+parts.Boundary())
	if _, err := io.WriteString(&body, headerToString(header)); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(&body, "\r\n"); err != nil {
		return nil, err
	}
	textHeader := make(textproto.MIMEHeader)
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	textPart, err := parts.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(textPart, "Thank you for your purchase. Your invoice is attached as a PDF.\r\n"); err != nil {
		return nil, err
	}
	pdfHeader := make(textproto.MIMEHeader)
	pdfHeader.Set("Content-Type", "application/pdf")
	pdfHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, filename))
	pdfHeader.Set("Content-Transfer-Encoding", "base64")
	pdfPart, err := parts.CreatePart(pdfHeader)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(pdf)
	for len(encoded) > 76 {
		if _, err := io.WriteString(pdfPart, encoded[:76]+"\r\n"); err != nil {
			return nil, err
		}
		encoded = encoded[76:]
	}
	if _, err := io.WriteString(pdfPart, encoded+"\r\n"); err != nil {
		return nil, err
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func headerToString(header textproto.MIMEHeader) string {
	var output strings.Builder
	for key, values := range header {
		for _, value := range values {
			fmt.Fprintf(&output, "%s: %s\r\n", key, value)
		}
	}
	return output.String()
}
