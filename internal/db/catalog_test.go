package db

import (
	"strings"
	"testing"
)

func TestDemoProductDescriptionIncludesUsefulItemSpecificSections(t *testing.T) {
	description := demoProductDescription(
		"Cotton Casual Shirt",
		"Breathable regular-fit cotton shirt.",
		"Levis",
		"Fashion",
	)

	for _, section := range []string{
		"Overview",
		"Key features",
		"Specifications",
		"How to use",
		"Benefits",
		"Materials / ingredients",
		"Size / weight",
		"Care / storage",
		"Warranty",
	} {
		if !strings.Contains(description, section+"\n") {
			t.Errorf("description is missing section %q", section)
		}
	}

	for _, detail := range []string{
		"Overview\nCotton Casual Shirt by Levis is listed in the Fashion collection.",
		"Breathable regular-fit cotton shirt",
		"apparel",
		"cotton",
		"size chart",
		"No size, capacity, or weight measurement",
		"No warranty duration",
	} {
		if !strings.Contains(description, detail) {
			t.Errorf("description is missing item-specific detail %q", detail)
		}
	}
	if strings.HasPrefix(description, "Overview\n ") || strings.HasPrefix(description, "Overview\n\t") {
		t.Fatal("overview should not contain source indentation")
	}
}

func TestDemoProductDescriptionExtractsListedSize(t *testing.T) {
	description := demoProductDescription(
		"Portable Power Bank 20000mAh",
		"Fast charging power bank with dual USB-C ports.",
		"Anker",
		"Electronics",
	)

	if !strings.Contains(description, "20000mAh") {
		t.Fatal("description did not carry over the listed battery capacity")
	}
	if !strings.Contains(description, "portable battery") {
		t.Fatal("description did not select the portable-battery usage profile")
	}
}
