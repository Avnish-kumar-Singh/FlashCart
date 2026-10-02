package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestValidCategoryNormalizesNameAndSlug(t *testing.T) {
	req := categoryRequest{Name: "  Home Audio  ", Slug: " Home-Audio ", SortOrder: 2}
	if !validCategory(&req, httptest.NewRecorder()) {
		t.Fatal("expected category to validate")
	}
	if req.Name != "Home Audio" || req.Slug != "home-audio" {
		t.Fatalf("category was not normalized: name=%q slug=%q", req.Name, req.Slug)
	}
}

func TestValidCategoryRejectsInvalidSlugAndSortOrder(t *testing.T) {
	for _, req := range []categoryRequest{
		{Name: "Audio", Slug: "not a slug"},
		{Name: "Audio", Slug: "audio", SortOrder: -1},
	} {
		if validCategory(&req, httptest.NewRecorder()) {
			t.Fatalf("expected invalid category to be rejected: %+v", req)
		}
	}
}
