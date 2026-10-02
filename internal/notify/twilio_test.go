package notify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestTwilioSenderUsesMessagingServiceSidWhenConfigured(t *testing.T) {
	var gotValues map[string]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		gotValues = map[string]string{
			"To":                  r.PostForm.Get("To"),
			"From":                r.PostForm.Get("From"),
			"Body":                r.PostForm.Get("Body"),
			"MessagingServiceSid": r.PostForm.Get("MessagingServiceSid"),
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(`{"sid":"SM123"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	sender := &TwilioSender{
		AccountSID:          "AC123",
		AuthToken:           "token",
		FromNumber:          "+15017122661",
		CountryCode:         "+91",
		MessagingServiceSID: "MG123",
		HTTPClient:          client,
	}

	if err := sender.Send(context.Background(), ChannelSMS, Recipient{Phone: "+919876543210"}, "Your code is 123456"); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if gotValues["From"] != "" {
		t.Fatalf("expected no From parameter when MessagingServiceSid is configured, got %q", gotValues["From"])
	}
	if gotValues["MessagingServiceSid"] != "MG123" {
		t.Fatalf("expected MessagingServiceSid to be MG123, got %q", gotValues["MessagingServiceSid"])
	}
	if gotValues["Body"] != "Your code is 123456" {
		t.Fatalf("expected body to be preserved, got %q", gotValues["Body"])
	}
}

func TestTwilioSenderUsesFromNumberWhenMessagingServiceNotConfigured(t *testing.T) {
	var gotValues map[string]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		gotValues = map[string]string{
			"To":   r.PostForm.Get("To"),
			"From": r.PostForm.Get("From"),
			"Body": r.PostForm.Get("Body"),
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(`{"sid":"SM456"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	sender := &TwilioSender{
		AccountSID:  "AC123",
		AuthToken:   "token",
		FromNumber:  "+15017122661",
		CountryCode: "+91",
		HTTPClient:  client,
	}

	if err := sender.Send(context.Background(), ChannelSMS, Recipient{Phone: "+919876543210"}, "Your code is 654321"); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if gotValues["From"] != "+15017122661" {
		t.Fatalf("expected From to be +15017122661, got %q", gotValues["From"])
	}
	if gotValues["Body"] != "Your code is 654321" {
		t.Fatalf("expected body to be preserved, got %q", gotValues["Body"])
	}
}
