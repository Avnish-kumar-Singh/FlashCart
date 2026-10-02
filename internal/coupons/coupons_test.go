package coupons

import "testing"

func TestEvaluateCategoryCouponDiscountsOnlyMatchingItems(t *testing.T) {
	rule := Rule{Active: true, DiscountType: "PERCENT", DiscountValue: 10, MinOrderPaise: 50000, EligibleCategory: "Snacks"}
	result := Evaluate(rule, []Line{
		{Category: "Snacks", UnitPricePaise: 10000, Quantity: 2},
		{Category: "Electronics", UnitPricePaise: 80000, Quantity: 1},
	}, 100000, false, "", "")
	if !result.Eligible || result.EligiblePaise != 20000 || result.DiscountPaise != 2000 {
		t.Fatalf("unexpected category offer calculation: %+v", result)
	}
}

func TestEvaluateRejectsMinimumAndNewUserRules(t *testing.T) {
	rule := Rule{Active: true, DiscountType: "FIXED", DiscountValue: 5000, MinOrderPaise: 50000, NewUserOnly: true}
	if result := Evaluate(rule, nil, 49999, true, "", ""); result.Eligible || result.Reason == "" {
		t.Fatalf("expected cart minimum rejection, got %+v", result)
	}
	if result := Evaluate(rule, nil, 60000, false, "", ""); result.Eligible || result.Reason == "" {
		t.Fatalf("expected new-customer rejection, got %+v", result)
	}
}

func TestEvaluateEnforcesPaymentIssuerAndCapsDiscount(t *testing.T) {
	rule := Rule{Active: true, DiscountType: "PERCENT", DiscountValue: 15, MaxDiscountPaise: 1000, MinOrderPaise: 1, RequiredPaymentMethod: "CARD", RequiredIssuer: "SBI"}
	lines := []Line{{Category: "Mobiles", UnitPricePaise: 10000, Quantity: 1}}
	if result := Evaluate(rule, lines, 10000, false, "WALLET", "SBI"); result.Eligible {
		t.Fatalf("wrong payment rail unexpectedly eligible: %+v", result)
	}
	result := Evaluate(rule, lines, 10000, false, "CARD", "SBI")
	if !result.Eligible || result.DiscountPaise != 1000 {
		t.Fatalf("issuer offer or maximum discount was not applied: %+v", result)
	}
}
