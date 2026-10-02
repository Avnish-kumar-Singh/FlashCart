// Package notify stands in for a real email/WhatsApp provider (SendGrid,
// SES, Twilio, WhatsApp Business API) the same way internal/payment
// simulates a payment gateway: no real messages are sent, but the
// interface is shaped so swapping in a real provider later only means
// writing a new implementation of Sender, not touching any caller.
package notify

import (
	"context"
	"log"
)

type Channel string

const (
	ChannelEmail    Channel = "EMAIL"
	ChannelSMS      Channel = "SMS"
	ChannelWhatsApp Channel = "WHATSAPP"
	ChannelBoth     Channel = "BOTH"
)

type Recipient struct {
	Email string
	Phone string
}

type Sender interface {
	// Send simulates delivering `message` to `recipient` over `channel`.
	// A real implementation would call out to a provider SDK here and
	// return its error; this one just logs, matching payment.Simulator's
	// "latency + outcome, no real side effect" shape.
	Send(ctx context.Context, channel Channel, recipient Recipient, message string) error
}

type LogSender struct{}

func NewLogSender() *LogSender { return &LogSender{} }

func (s *LogSender) Send(ctx context.Context, channel Channel, recipient Recipient, message string) error {
	switch channel {
	case ChannelEmail:
		log.Printf("[EMAIL SIMULATED] -> %s: %q", recipient.Email, message)
	case ChannelSMS:
		target := recipient.Phone
		if target == "" {
			target = "(no phone on file, would skip in production)"
		}
		log.Printf("[SMS SIMULATED] -> %s: %q", target, message)
	case ChannelWhatsApp:
		target := recipient.Phone
		if target == "" {
			target = "(no phone on file, would skip in production)"
		}
		log.Printf("[WHATSAPP SIMULATED] -> %s: %q", target, message)
	case ChannelBoth:
		_ = s.Send(ctx, ChannelEmail, recipient, message)
		_ = s.Send(ctx, ChannelSMS, recipient, message)
	}
	return nil
}
