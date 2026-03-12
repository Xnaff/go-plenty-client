package mock

import (
	"context"
	"testing"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time check that Provider implements Generator.
var _ generate.Generator = (*Provider)(nil)

func TestProviderName(t *testing.T) {
	p := &Provider{}
	if got := p.Name(); got != "mock" {
		t.Errorf("Name() = %q, want %q", got, "mock")
	}
}

func TestGenerateProductTexts(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	for _, lang := range generate.SupportedLanguages {
		t.Run(lang, func(t *testing.T) {
			req := generate.ProductTextRequest{
				ProductType: "electronics",
				ProductName: "Test Widget",
				Category:    "Gadgets",
				Language:    lang,
			}

			texts, err := p.GenerateProductTexts(ctx, req)
			if err != nil {
				t.Fatalf("GenerateProductTexts(%s) error: %v", lang, err)
			}

			if texts.Name == "" {
				t.Error("Name is empty")
			}
			if texts.ShortDescription == "" {
				t.Error("ShortDescription is empty")
			}
			if texts.Description == "" {
				t.Error("Description is empty")
			}
			if texts.TechnicalData == "" {
				t.Error("TechnicalData is empty")
			}
			if texts.MetaDescription == "" {
				t.Error("MetaDescription is empty")
			}
			if texts.URLContent == "" {
				t.Error("URLContent is empty")
			}
			if texts.PreviewText == "" {
				t.Error("PreviewText is empty")
			}
		})
	}
}

func TestGeneratePropertyValues_Text(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "electronics",
		ProductName: "Test Widget",
		Language:    "en",
		Properties: []generate.PropertySpec{
			{ID: 1, Name: "Material", PropertyType: "text"},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}
	if len(result.Values) != 1 {
		t.Fatalf("got %d values, want 1", len(result.Values))
	}

	v := result.Values[0]
	if v.PropertyID != 1 {
		t.Errorf("PropertyID = %d, want 1", v.PropertyID)
	}
	if v.TextValue == "" {
		t.Error("TextValue is empty for text property")
	}
	if v.IntValue != nil {
		t.Error("IntValue should be nil for text property")
	}
	if v.FloatValue != nil {
		t.Error("FloatValue should be nil for text property")
	}
}

func TestGeneratePropertyValues_Int(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "electronics",
		Language:    "en",
		Properties: []generate.PropertySpec{
			{ID: 2, Name: "Weight", PropertyType: "int"},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}

	v := result.Values[0]
	if v.PropertyID != 2 {
		t.Errorf("PropertyID = %d, want 2", v.PropertyID)
	}
	if v.IntValue == nil {
		t.Fatal("IntValue is nil for int property")
	}
	if *v.IntValue != 42 {
		t.Errorf("IntValue = %d, want 42", *v.IntValue)
	}
	if v.TextValue != "" {
		t.Error("TextValue should be empty for int property")
	}
}

func TestGeneratePropertyValues_Float(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "electronics",
		Language:    "en",
		Properties: []generate.PropertySpec{
			{ID: 3, Name: "Price", PropertyType: "float"},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}

	v := result.Values[0]
	if v.PropertyID != 3 {
		t.Errorf("PropertyID = %d, want 3", v.PropertyID)
	}
	if v.FloatValue == nil {
		t.Fatal("FloatValue is nil for float property")
	}
	if *v.FloatValue != 9.99 {
		t.Errorf("FloatValue = %f, want 9.99", *v.FloatValue)
	}
	if v.TextValue != "" {
		t.Error("TextValue should be empty for float property")
	}
}

func TestGeneratePropertyValues_Selection(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "electronics",
		Language:    "en",
		Properties: []generate.PropertySpec{
			{ID: 4, Name: "Color", PropertyType: "selection", Options: []string{"Red", "Blue", "Green"}},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}

	v := result.Values[0]
	if v.PropertyID != 4 {
		t.Errorf("PropertyID = %d, want 4", v.PropertyID)
	}
	if v.SelectionValue != "Red" {
		t.Errorf("SelectionValue = %q, want %q", v.SelectionValue, "Red")
	}
}

func TestGeneratePropertyValues_SelectionEmptyOptions(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "electronics",
		Language:    "en",
		Properties: []generate.PropertySpec{
			{ID: 5, Name: "Size", PropertyType: "selection", Options: []string{}},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}

	v := result.Values[0]
	if v.SelectionValue != "" {
		t.Errorf("SelectionValue = %q, want empty string for empty options", v.SelectionValue)
	}
}

func TestGeneratePropertyValues_MultipleProperties(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	req := generate.PropertyValueRequest{
		ProductType: "fashion",
		ProductName: "Designer Shirt",
		Language:    "de",
		Properties: []generate.PropertySpec{
			{ID: 10, Name: "Material", PropertyType: "text"},
			{ID: 11, Name: "Weight", PropertyType: "int"},
			{ID: 12, Name: "Rating", PropertyType: "float"},
			{ID: 13, Name: "Color", PropertyType: "selection", Options: []string{"Schwarz", "Weiss"}},
		},
	}

	result, err := p.GeneratePropertyValues(ctx, req)
	if err != nil {
		t.Fatalf("GeneratePropertyValues error: %v", err)
	}

	if len(result.Values) != 4 {
		t.Fatalf("got %d values, want 4", len(result.Values))
	}

	// Verify each type is correctly set
	if result.Values[0].TextValue == "" {
		t.Error("text property should have TextValue")
	}
	if result.Values[1].IntValue == nil {
		t.Error("int property should have IntValue")
	}
	if result.Values[2].FloatValue == nil {
		t.Error("float property should have FloatValue")
	}
	if result.Values[3].SelectionValue != "Schwarz" {
		t.Errorf("selection property SelectionValue = %q, want %q", result.Values[3].SelectionValue, "Schwarz")
	}
}
