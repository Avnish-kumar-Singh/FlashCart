package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const groqChatCompletionsURL = "https://api.groq.com/openai/v1/chat/completions"

var (
	catalogWordPattern   = regexp.MustCompile(`[a-z0-9]+`)
	quotedProductPattern = regexp.MustCompile(`["“]([^"”]+)["”]`)
	catalogStopWords     = map[string]struct{}{
		"a": {}, "about": {}, "available": {}, "can": {}, "catalog": {}, "category": {},
		"compare": {}, "could": {}, "details": {}, "do": {}, "does": {}, "find": {},
		"for": {}, "get": {}, "have": {}, "in": {}, "is": {}, "it": {}, "me": {},
		"of": {}, "on": {}, "or": {}, "please": {}, "product": {}, "products": {},
		"search": {}, "show": {}, "tell": {}, "the": {}, "there": {}, "this": {},
		"to": {}, "want": {}, "what": {}, "with": {}, "you": {},
	}
)

type AIShoppingHandler struct {
	DB       *pgxpool.Pool
	APIKey   string
	Model    string
	Endpoint string
	Client   *http.Client
}

type aiConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiSuggestionRequest struct {
	Messages    []aiConversationMessage `json:"messages"`
	SearchQuery string                  `json:"search_query"`
	ProductID   string                  `json:"product_id"`
}

type aiCatalogProduct struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Brand          string `json:"brand"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	PricePaise     int64  `json:"price_paise"`
	Stock          int    `json:"stock"`
	MatchedTerms   int    `json:"-"`
	ExactNameMatch bool   `json:"-"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqCompletionRequest struct {
	Model               string        `json:"model"`
	Messages            []groqMessage `json:"messages"`
	Temperature         float64       `json:"temperature"`
	MaxCompletionTokens int           `json:"max_completion_tokens"`
}

type groqCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (h *AIShoppingHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12*1024)
	var input aiSuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid assistant request")
		return
	}
	if err := validateAIRequest(&input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(h.APIKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "AI suggestions are not configured. Set GROQ_API_KEY on the API server.")
		return
	}

	userContext := make([]string, 0, 3)
	for i := len(input.Messages) - 1; i >= 0 && len(userContext) < 3; i-- {
		if input.Messages[i].Role == "user" {
			userContext = append(userContext, input.Messages[i].Content)
		}
	}
	searchQuery := strings.TrimSpace(strings.Join(append([]string{input.SearchQuery}, userContext...), " "))

	products, err := h.catalogContext(r.Context(), strings.TrimSpace(input.ProductID), searchQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load catalog context")
		return
	}
	selectedProduct := selectDirectCatalogProduct(products, strings.TrimSpace(input.ProductID), catalogSearchTerms(searchQuery))

	encodedProducts, _ := json.Marshal(products)
	model := strings.TrimSpace(h.Model)
	if model == "" {
		model = "openai/gpt-oss-120b"
	}

	messages := []groqMessage{
		{
			Role: "system",
			Content: `You are FlashCart's shopping assistant. Help shoppers compare products and understand the current catalog.
The following catalog snapshot contains records retrieved from the live products table. Every product object included in it is confirmed to exist in the catalog. Match shopper wording against product names, brands, categories, and listing descriptions; a product present in the snapshot must never be described as absent. When a matching record exists, directly answer availability and quote its listed price and stock accurately. If there is no matching record, say no matching listing was found for the requested wording; do not claim the entire catalog is empty. Give alternatives only when no direct match exists or the shopper asks for them.
Use only facts in the catalog context and conversation. Do not invent specifications, prices, stock, warranty terms, personal experience, ratings, or product availability. Clearly label general advice and any inference as such. If a requested product is absent from the retrieved matches, do not pretend to know its current specifications.
This application has no verified customer-review records or review feed. When asked what customers say or to summarize reviews, clearly state that verified reviews are not available in this store and never fabricate review sentiment or repeat demo ratings as real feedback.
When useful, organize replies with short plain-text headings such as Catalog match, What the listing says, Pros, Trade-offs, Alternatives, Value, and Reviews. Use concise bullets, explain when a comparison is based only on listing data, and ask one focused follow-up question when the shopper's needs are unclear. Never claim to browse the live web.`,
		},
		{
			Role:    "system",
			Content: "Current catalog snapshot (JSON; price is in paise): " + string(encodedProducts),
		},
	}
	if selectedProduct != nil {
		selectedJSON, _ := json.Marshal(selectedProduct)
		messages = append(messages, groqMessage{
			Role:    "system",
			Content: "VERIFIED SELECTED PRODUCT: This exact product was retrieved from the live catalog using the current product page ID or a direct product-name match. It is definitely present. Do not ask which product is being viewed or claim that it is missing. Use these exact facts first: " + string(selectedJSON),
		})
	}
	for _, message := range input.Messages {
		messages = append(messages, groqMessage{Role: message.Role, Content: message.Content})
	}

	answer, err := h.complete(r.Context(), model, messages)
	if err != nil {
		writeError(w, http.StatusBadGateway, "The AI service could not respond. Please try again shortly.")
		return
	}
	if selectedProduct != nil {
		answer = enforceVerifiedCatalogMatch(answer, *selectedProduct)
	}
	writeJSON(w, http.StatusOK, map[string]string{"suggestion": answer, "model": model})
}

func selectDirectCatalogProduct(products []aiCatalogProduct, productID string, terms []string) *aiCatalogProduct {
	for i := range products {
		if productID != "" && products[i].ID == productID {
			return &products[i]
		}
	}
	for i := range products {
		if products[i].ExactNameMatch {
			return &products[i]
		}
	}
	minimumMatches := minimumCatalogTermMatches(len(terms))
	for i := range products {
		if len(terms) > 0 && products[i].MatchedTerms >= minimumMatches {
			return &products[i]
		}
	}
	return nil
}

func enforceVerifiedCatalogMatch(answer string, product aiCatalogProduct) string {
	lowerAnswer := strings.NewReplacer("’", "'", "‘", "'").Replace(strings.ToLower(answer))
	for _, claim := range []string{
		"no matching listing",
		"not present in the current catalog",
		"not present in the catalog",
		"not in the catalog",
		"not currently listed",
		"i'm not sure which item",
		"i am not sure which item",
		"don't have any details about that item",
		"do not have any details about that item",
		"could you tell me the product name",
		"could you tell me the name or a brief description",
	} {
		if strings.Contains(lowerAnswer, claim) {
			return verifiedCatalogProductSummary(product)
		}
	}
	return answer
}

func verifiedCatalogProductSummary(product aiCatalogProduct) string {
	availability := fmt.Sprintf("%d units in stock", product.Stock)
	if product.Stock == 0 {
		availability = "currently out of stock"
	}
	return fmt.Sprintf("Catalog match\n- %s by %s (%s)\n- Price: ₹%.2f\n- Availability: %s\n\nWhat the listing says\n%s\n\nThis listing is confirmed in the current catalog. Ask about a specific feature or request a comparison for more guidance.", product.Name, product.Brand, product.Category, float64(product.PricePaise)/100, availability, product.Description)
}

func validateAIRequest(input *aiSuggestionRequest) error {
	if len(input.Messages) == 0 || len(input.Messages) > 8 {
		return fmt.Errorf("send between 1 and 8 conversation messages")
	}
	if len(input.ProductID) > 64 || len(input.SearchQuery) > 500 {
		return fmt.Errorf("product or search context is too long")
	}
	for i := range input.Messages {
		message := &input.Messages[i]
		message.Content = strings.TrimSpace(message.Content)
		if message.Role != "user" && message.Role != "assistant" {
			return fmt.Errorf("conversation contains an unsupported message role")
		}
		if message.Content == "" {
			return fmt.Errorf("each message must contain between 1 and 1500 characters")
		}
		if message.Role == "user" && len(message.Content) > 1500 {
			return fmt.Errorf("shopper messages must not exceed 1500 characters")
		}
		if message.Role == "assistant" && len(message.Content) > 8000 {
			return fmt.Errorf("assistant history message is too long")
		}
	}
	if input.Messages[len(input.Messages)-1].Role != "user" {
		return fmt.Errorf("the latest conversation message must be from the shopper")
	}
	return nil
}

func (h *AIShoppingHandler) catalogContext(ctx context.Context, productID, query string) ([]aiCatalogProduct, error) {
	searchTerms := catalogSearchTerms(query)
	normalizedQuery := strings.Join(searchTerms, " ")
	category := aiCategoryHint(query)
	rows, err := h.DB.Query(ctx, `
		SELECT p.id::text, p.name, p.brand, p.category, p.description, p.price_paise, p.stock,
		       match_score.matched_terms, normalized.normalized_name = $2
		FROM products p
		CROSS JOIN LATERAL (
			SELECT regexp_replace(lower(concat_ws(' ', p.name, p.brand, p.category, p.description)), '[^a-z0-9]+', ' ', 'g') AS search_text,
			       regexp_replace(lower(p.name), '[^a-z0-9]+', ' ', 'g') AS normalized_name
		) normalized
		CROSS JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE normalized.search_text LIKE '%' || term || '%')::int AS matched_terms
			FROM unnest($4::text[]) AS terms(term)
		) match_score
		WHERE ($1 <> '' AND p.id::text = $1)
		   OR ($2 <> '' AND (normalized.normalized_name = $2 OR normalized.normalized_name LIKE '%' || $2 || '%' OR p.brand ILIKE '%' || $2 || '%' OR p.description ILIKE '%' || $2 || '%'))
		   OR (cardinality($4::text[]) > 0 AND match_score.matched_terms >= $5)
		   OR ($3 <> '' AND p.category = $3)
		   OR ($1 <> '' AND p.category = (SELECT focus.category FROM products focus WHERE focus.id::text = $1 LIMIT 1))
		   OR ($1 = '' AND $2 = '' AND $3 = '')
		ORDER BY CASE WHEN p.id::text = $1 THEN 0
		              WHEN normalized.normalized_name = $2 THEN 1
		              WHEN $2 <> '' AND (normalized.normalized_name LIKE '%' || $2 || '%' OR p.brand ILIKE '%' || $2 || '%' OR p.description ILIKE '%' || $2 || '%') THEN 2
		              ELSE 3 END,
		         match_score.matched_terms DESC,
		         p.created_at DESC
		LIMIT 8`, productID, normalizedQuery, category, searchTerms, minimumCatalogTermMatches(len(searchTerms)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := make([]aiCatalogProduct, 0, 8)
	for rows.Next() {
		var product aiCatalogProduct
		if err := rows.Scan(&product.ID, &product.Name, &product.Brand, &product.Category, &product.Description, &product.PricePaise, &product.Stock, &product.MatchedTerms, &product.ExactNameMatch); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	hasDirectMatch := false
	for _, product := range products {
		if product.ID == productID || product.ExactNameMatch || (len(searchTerms) > 0 && product.MatchedTerms >= minimumCatalogTermMatches(len(searchTerms))) {
			hasDirectMatch = true
			break
		}
	}
	if hasDirectMatch {
		matched := make([]aiCatalogProduct, 0, len(products))
		for _, product := range products {
			if product.ID == productID || product.ExactNameMatch || product.MatchedTerms > 0 {
				matched = append(matched, product)
			}
		}
		products = matched
	}
	return products, nil
}

func catalogSearchTerms(query string) []string {
	query = strings.ToLower(query)
	if match := quotedProductPattern.FindStringSubmatch(query); len(match) == 2 {
		query = match[1]
	}

	words := catalogWordPattern.FindAllString(query, -1)
	terms := make([]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		if _, stopWord := catalogStopWords[word]; stopWord {
			continue
		}
		if _, duplicate := seen[word]; duplicate {
			continue
		}
		seen[word] = struct{}{}
		terms = append(terms, word)
		if len(terms) == 12 {
			break
		}
	}
	return terms
}

func minimumCatalogTermMatches(termCount int) int {
	if termCount == 0 {
		return 0
	}
	return (termCount*3 + 4) / 5
}

func (h *AIShoppingHandler) complete(ctx context.Context, model string, messages []groqMessage) (string, error) {
	requestBody, err := json.Marshal(groqCompletionRequest{
		Model:               model,
		Messages:            messages,
		Temperature:         0.3,
		MaxCompletionTokens: 900,
	})
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimSpace(h.Endpoint)
	if endpoint == "" {
		endpoint = groqChatCompletionsURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+h.APIKey)
	request.Header.Set("Content-Type", "application/json")

	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 40 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Groq returned status %d", response.StatusCode)
	}

	var completion groqCompletionResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&completion); err != nil {
		return "", err
	}
	if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("Groq returned an empty completion")
	}
	return strings.TrimSpace(completion.Choices[0].Message.Content), nil
}

func aiCategoryHint(query string) string {
	query = strings.ToLower(query)
	categories := []struct {
		category string
		terms    []string
	}{
		{"Mobiles", []string{"iphone", "phone", "smartphone", "mobile", "android", "pixel", "galaxy"}},
		{"Electronics", []string{"laptop", "computer", "headphone", "earbud", "monitor", "camera", "speaker", "router", "tablet", "keyboard"}},
		{"Fashion", []string{"shirt", "dress", "shoe", "fashion", "jeans", "sneaker", "clothing"}},
		{"Appliances", []string{"appliance", "washing machine", "refrigerator", "mixer", "air fryer", "kettle", "oven", "iron", "vacuum", "cooler"}},
		{"Home", []string{"home", "lamp", "bed", "curtain", "furniture", "decor", "pillow"}},
		{"Beauty", []string{"beauty", "skin", "serum", "shampoo", "makeup", "face", "sunscreen"}},
		{"Grocery", []string{"grocery", "food", "coffee", "tea", "rice", "snack", "oats", "honey"}},
	}
	for _, item := range categories {
		for _, term := range item.terms {
			if strings.Contains(query, term) {
				return item.category
			}
		}
	}
	return ""
}
