package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAICategoryHintCoversCatalogCategories(t *testing.T) {
	for _, test := range []struct {
		query    string
		category string
	}{
		{"iPhone 16", "Mobiles"},
		{"gaming laptop", "Electronics"},
		{"cotton shirt", "Fashion"},
		{"air fryer", "Appliances"},
		{"bedroom lamp", "Home"},
		{"vitamin c serum", "Beauty"},
		{"coffee beans", "Grocery"},
	} {
		if got := aiCategoryHint(test.query); got != test.category {
			t.Errorf("aiCategoryHint(%q) = %q, want %q", test.query, got, test.category)
		}
	}
}

func TestCatalogSearchTermsExtractProductFromNaturalLanguageQuestion(t *testing.T) {
	terms := catalogSearchTerms(`Do you have "Green Tea Bags 100 Count"?`)
	want := []string{"green", "tea", "bags", "100", "count"}
	if strings.Join(terms, " ") != strings.Join(want, " ") {
		t.Fatalf("catalogSearchTerms() = %v, want %v", terms, want)
	}
}

func TestCatalogSearchTermsKeepProductTermsAndDropSearchWording(t *testing.T) {
	terms := catalogSearchTerms("Search: green teas bag 100-count; Category: Grocery")
	want := []string{"green", "teas", "bag", "100", "count", "grocery"}
	if strings.Join(terms, " ") != strings.Join(want, " ") {
		t.Fatalf("catalogSearchTerms() = %v, want %v", terms, want)
	}
	if minimumCatalogTermMatches(len(terms)) != 4 {
		t.Fatalf("minimumCatalogTermMatches(%d) = %d, want 4", len(terms), minimumCatalogTermMatches(len(terms)))
	}
}

func TestAIShoppingHandlerRequiresAPIKey(t *testing.T) {
	handler := &AIShoppingHandler{}
	request := httptest.NewRequest(http.MethodPost, "/ai/suggestions", strings.NewReader(`{"messages":[{"role":"user","content":"Compare phones"}]}`))
	response := httptest.NewRecorder()

	handler.Suggest(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), "GROQ_API_KEY") {
		t.Fatal("missing API key response should explain how to configure Groq")
	}
}

func TestGroqCompletionUsesConfiguredModelAndSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/v1/chat/completions" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-groq-key" {
			t.Errorf("unexpected authorization header")
		}
		var payload groqCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode completion request: %v", err)
		}
		if payload.Model != "openai/gpt-oss-120b" {
			t.Errorf("model = %q", payload.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Catalog-grounded answer"}}]}`))
	}))
	defer server.Close()

	handler := &AIShoppingHandler{
		APIKey:   "test-groq-key",
		Endpoint: server.URL + "/openai/v1/chat/completions",
		Client:   server.Client(),
	}
	ctx := context.Background()
	answer, err := handler.complete(ctx, "openai/gpt-oss-120b", []groqMessage{{Role: "user", Content: "Compare phones"}})
	if err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if answer != "Catalog-grounded answer" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestValidateAIRequestAllowsDetailedAssistantHistory(t *testing.T) {
	input := aiSuggestionRequest{
		Messages: []aiConversationMessage{
			{Role: "assistant", Content: strings.Repeat("Detailed product guidance. ", 100)},
			{Role: "user", Content: "Tell me more"},
		},
	}
	if err := validateAIRequest(&input); err != nil {
		t.Fatalf("long assistant history should be allowed: %v", err)
	}
}

func TestValidateAIRequestStillLimitsShopperMessageLength(t *testing.T) {
	input := aiSuggestionRequest{
		Messages: []aiConversationMessage{
			{Role: "user", Content: strings.Repeat("x", 1501)},
		},
	}
	if err := validateAIRequest(&input); err == nil {
		t.Fatal("expected an oversized shopper message to be rejected")
	}
}

func TestEnforceVerifiedCatalogMatchCorrectsFalseNoMatch(t *testing.T) {
	product := aiCatalogProduct{
		Name:        "Wireless Gaming Headset",
		Brand:       "HyperX",
		Category:    "Electronics",
		Description: "Low-latency audio with a detachable noise-cancelling mic.",
		PricePaise:  899900,
		Stock:       32,
	}
	answer := enforceVerifiedCatalogMatch("No matching listing was found for the product you're viewing.", product)

	for _, expected := range []string{
		"Wireless Gaming Headset",
		"HyperX",
		"₹8999.00",
		"32 units in stock",
		"detachable noise-cancelling mic",
	} {
		if !strings.Contains(answer, expected) {
			t.Errorf("corrected answer is missing %q: %s", expected, answer)
		}
	}
	if strings.Contains(answer, "No matching listing") {
		t.Fatal("verified catalog item must not retain a false no-match claim")
	}
}
