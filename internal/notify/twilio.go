package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// TwilioSender sends real SMS via the Twilio Messages REST API
// (https://api.twilio.com/2010-04-01/Accounts/{Sid}/Messages.json).
// It implements the same Sender interface as LogSender, so it's a
// drop-in replacement — see NewSender in router wiring.
type TwilioSender struct {
	AccountSID  string
	AuthToken   string
	FromNumber  string // must be an SMS-capable Twilio number, e.g. "+15017122661"
	CountryCode string // prepended to numbers that don't start with '+', e.g. "+91"

	// MessagingServiceSID is the preferred sender for Twilio trial accounts
	// and accounts using approved content templates for OTPs.
	MessagingServiceSID string
	HTTPClient          *http.Client

	// Fallback used for EMAIL (Twilio doesn't send email) and for
	// WHATSAPP if no WhatsApp-approved sender is configured.
	Fallback Sender
}

func NewTwilioSender(accountSID, authToken, fromNumber, countryCode, messagingServiceSID string, fallback Sender) *TwilioSender {
	if fallback == nil {
		fallback = NewLogSender()
	}
	return &TwilioSender{
		AccountSID:          accountSID,
		AuthToken:           authToken,
		FromNumber:          fromNumber,
		CountryCode:         countryCode,
		MessagingServiceSID: messagingServiceSID,
		HTTPClient:          &http.Client{},
		Fallback:            fallback,
	}
}

var digitsOnly = regexp.MustCompile(`[^\d+]`)

// toE164 normalizes a raw phone number to E.164 (e.g. "+919876543210").
// This is the single most common reason a "sent" SMS never arrives:
// Twilio silently accepts a malformed destination in some cases, or
// rejects it with error 21211/21614, and without E.164 the local "To"
// number is ambiguous about country.
func (t *TwilioSender) toE164(raw string) string {
	cleaned := digitsOnly.ReplaceAllString(strings.TrimSpace(raw), "")
	if strings.HasPrefix(cleaned, "+") {
		return cleaned
	}
	// Indian mobile numbers are commonly typed as 10 digits (98765 43210)
	// or with a leading 0 (098765 43210) — strip a single leading 0 before
	// adding the country code so we don't produce "+910..." with an extra
	// digit.
	cleaned = strings.TrimPrefix(cleaned, "0")
	cc := t.CountryCode
	if cc == "" {
		cc = "+91"
	}
	if !strings.HasPrefix(cc, "+") {
		cc = "+" + cc
	}
	return cc + cleaned
}

func (t *TwilioSender) Send(ctx context.Context, channel Channel, recipient Recipient, message string) error {
	switch channel {
	case ChannelSMS:
		return t.sendSMS(ctx, recipient.Phone, message)
	case ChannelEmail:
		return t.Fallback.Send(ctx, ChannelEmail, recipient, message)
	case ChannelWhatsApp:
		// Twilio WhatsApp needs a separate, Meta-approved sender (and a
		// pre-approved template outside a 24h session window), which is
		// a different onboarding flow from plain SMS. Fall back rather
		// than silently mis-send.
		return t.Fallback.Send(ctx, ChannelWhatsApp, recipient, message)
	case ChannelBoth:
		emailErr := t.Fallback.Send(ctx, ChannelEmail, recipient, message)
		smsErr := t.sendSMS(ctx, recipient.Phone, message)
		if smsErr != nil {
			return smsErr
		}
		return emailErr
	}
	return nil
}

func (t *TwilioSender) sendSMS(ctx context.Context, phone, message string) error {
	if t.AccountSID == "" || t.AuthToken == "" {
		return fmt.Errorf("twilio: TWILIO_ACCOUNT_SID / TWILIO_AUTH_TOKEN not configured")
	}
	if t.MessagingServiceSID == "" && t.FromNumber == "" {
		return fmt.Errorf("twilio: TWILIO_FROM_NUMBER or TWILIO_MESSAGING_SERVICE_SID not configured")
	}
	if phone == "" {
		return fmt.Errorf("twilio: recipient has no phone number on file")
	}

	to := t.toE164(phone)

	form := url.Values{}
	form.Set("To", to)
	if t.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", t.MessagingServiceSID)
	} else {
		form.Set("From", t.FromNumber)
	}
	form.Set("Body", message)

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", t.AccountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(t.AccountSID, t.AuthToken)

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Twilio returns 201 Created with the message resource on success.
	// Anything else (400/401/403 etc.) means the message was NOT queued
	// for delivery — this is the difference between "our code thinks it
	// sent an SMS" and "a phone actually received one".
	if resp.StatusCode != http.StatusCreated {
		var twilioErr struct {
			Code     int    `json:"code"`
			Message  string `json:"message"`
			MoreInfo string `json:"more_info"`
		}
		_ = json.Unmarshal(body, &twilioErr)
		if twilioErr.Message != "" {
			if twilioErr.Code == 572006 {
				return fmt.Errorf("twilio: SMS to %s rejected (%d): %s — %s. Trial accounts require a verified recipient or a configured Messaging Service/template.", to, twilioErr.Code, twilioErr.Message, twilioErr.MoreInfo)
			}
			return fmt.Errorf("twilio: SMS to %s rejected (%d): %s — %s", to, twilioErr.Code, twilioErr.Message, twilioErr.MoreInfo)
		}
		return fmt.Errorf("twilio: SMS to %s rejected, status %d: %s", to, resp.StatusCode, string(body))
	}

	log.Printf("[TWILIO SMS] queued -> %s", to)
	return nil
}
