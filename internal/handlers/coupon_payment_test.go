package handlers

import "testing"

func TestNormalizeCouponIssuer(t *testing.T) {
	for input, want := range map[string]string{
		"SBIN": "SBI", "STATE BANK OF INDIA": "SBI", "HDFC BANK": "HDFC",
		"ICIC": "ICICI", "UTIB": "AXIS", "PHONEPE WALLET": "PHONEPE",
	} {
		if got := normalizeCouponIssuer(input); got != want {
			t.Errorf("normalizeCouponIssuer(%q) = %q, want %q", input, got, want)
		}
	}
}
