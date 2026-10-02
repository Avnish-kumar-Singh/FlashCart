package coupons

import (
	"strings"
)

type Rule struct {
	ID                    string
	Code                  string
	Description           string
	OfferType             string
	DiscountType          string
	DiscountValue         int64
	MinOrderPaise         int64
	MaxDiscountPaise      int64
	MaxUses               int
	UsedCount             int
	EligibleCategory      string
	RequiredPaymentMethod string
	RequiredIssuer        string
	NewUserOnly           bool
	Active                bool
}

type Line struct {
	Category       string
	UnitPricePaise int64
	Quantity       int
}

type Result struct {
	Eligible      bool   `json:"eligible"`
	Reason        string `json:"reason,omitempty"`
	DiscountPaise int64  `json:"discount_paise"`
	EligiblePaise int64  `json:"eligible_paise"`
}

func Evaluate(rule Rule, lines []Line, subtotal int64, isNewUser bool, paymentMethod, issuer string) Result {
	result := Result{}
	if !rule.Active {
		result.Reason = "This offer is no longer active."
		return result
	}
	if rule.MaxUses > 0 && rule.UsedCount >= rule.MaxUses {
		result.Reason = "This offer has reached its redemption limit."
		return result
	}
	if subtotal < rule.MinOrderPaise {
		result.Reason = "Add more items to meet the minimum cart value."
		return result
	}
	if rule.NewUserOnly && !isNewUser {
		result.Reason = "This offer is for first-time customers."
		return result
	}
	if rule.RequiredPaymentMethod != "" && !strings.EqualFold(rule.RequiredPaymentMethod, paymentMethod) {
		result.Reason = "Choose the required payment method to use this offer."
		return result
	}
	if rule.RequiredIssuer != "" && !strings.EqualFold(rule.RequiredIssuer, issuer) {
		result.Reason = "Choose the matching card issuer or wallet to use this offer."
		return result
	}

	result.EligiblePaise = subtotal
	if rule.EligibleCategory != "" {
		result.EligiblePaise = 0
		for _, line := range lines {
			if strings.EqualFold(strings.TrimSpace(line.Category), strings.TrimSpace(rule.EligibleCategory)) {
				result.EligiblePaise += line.UnitPricePaise * int64(line.Quantity)
			}
		}
		if result.EligiblePaise == 0 {
			result.Reason = "Your cart has no items in this offer's category."
			return result
		}
	}

	switch rule.DiscountType {
	case "PERCENT":
		result.DiscountPaise = result.EligiblePaise * rule.DiscountValue / 100
	case "FIXED":
		result.DiscountPaise = rule.DiscountValue
	default:
		result.Reason = "This offer has an invalid discount configuration."
		return result
	}
	if rule.MaxDiscountPaise > 0 && result.DiscountPaise > rule.MaxDiscountPaise {
		result.DiscountPaise = rule.MaxDiscountPaise
	}
	if result.DiscountPaise > result.EligiblePaise {
		result.DiscountPaise = result.EligiblePaise
	}
	result.Eligible = result.DiscountPaise > 0
	if !result.Eligible {
		result.Reason = "This offer does not apply to the current cart."
	}
	return result
}
