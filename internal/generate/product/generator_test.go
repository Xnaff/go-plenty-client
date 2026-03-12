package product_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/janemig/plentyone/internal/generate"
	"github.com/janemig/plentyone/internal/generate/mock"
	"github.com/janemig/plentyone/internal/generate/product"
	"github.com/janemig/plentyone/internal/generate/validate"
)

func newTestGenerator() *product.Generator {
	provider := &mock.Provider{}
	validator := validate.NewValidator()
	languages := generate.SupportedLanguages
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return product.NewGenerator(provider, validator, languages, logger)
}

func TestGenerate_AllLanguages(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Niche:       "Consumer Electronics",
		Keywords:    []string{"wireless", "bluetooth"},
	}

	result, err := pg.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Texts) != len(generate.SupportedLanguages) {
		t.Fatalf("expected texts for %d languages, got %d", len(generate.SupportedLanguages), len(result.Texts))
	}

	for _, lang := range generate.SupportedLanguages {
		texts, ok := result.Texts[lang]
		if !ok {
			t.Errorf("missing texts for language %s", lang)
			continue
		}
		if texts.Name == "" {
			t.Errorf("empty name for language %s", lang)
		}
		if texts.Description == "" {
			t.Errorf("empty description for language %s", lang)
		}
		if texts.URLContent == "" {
			t.Errorf("empty urlContent for language %s", lang)
		}
		if texts.MetaDescription == "" {
			t.Errorf("empty metaDescription for language %s", lang)
		}
	}
}

func TestGenerate_WithProperties(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Properties: []generate.PropertySpec{
			{ID: 1, Name: "Material", PropertyType: "text"},
			{ID: 2, Name: "Weight", PropertyType: "int"},
			{ID: 3, Name: "Price", PropertyType: "float"},
			{ID: 4, Name: "Color", PropertyType: "selection", Options: []string{"red", "blue", "green"}},
		},
	}

	result, err := pg.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Non-text properties should be in result.Properties.
	if result.Properties == nil {
		t.Fatal("expected non-nil Properties for non-text property specs")
	}
	if len(result.Properties.Values) != 3 { // int, float, selection
		t.Errorf("expected 3 non-text property values, got %d", len(result.Properties.Values))
	}

	// Text properties should be in result.PropertyTexts, one per language.
	if result.PropertyTexts == nil {
		t.Fatal("expected non-nil PropertyTexts for text property specs")
	}
	if len(result.PropertyTexts) != len(generate.SupportedLanguages) {
		t.Errorf("expected PropertyTexts for %d languages, got %d", len(generate.SupportedLanguages), len(result.PropertyTexts))
	}
	for _, lang := range generate.SupportedLanguages {
		pv, ok := result.PropertyTexts[lang]
		if !ok {
			t.Errorf("missing PropertyTexts for language %s", lang)
			continue
		}
		if len(pv.Values) != 1 { // only "text" type
			t.Errorf("expected 1 text property value for %s, got %d", lang, len(pv.Values))
		}
	}
}

func TestGenerate_EmptyProperties(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Properties:  nil,
	}

	result, err := pg.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.Properties != nil {
		t.Error("expected nil Properties when no property specs provided")
	}
	if result.PropertyTexts != nil {
		t.Error("expected nil PropertyTexts when no property specs provided")
	}
}

func TestGenerateTexts_ReturnsSanitizedOutput(t *testing.T) {
	pg := newTestGenerator()
	req := generate.ProductTextRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Language:    "en",
	}

	texts, warnings, err := pg.GenerateTexts(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateTexts failed: %v", err)
	}

	// Mock data is well-formed, so it should pass validation cleanly.
	if texts.Name == "" {
		t.Error("expected non-empty name")
	}
	if texts.Description == "" {
		t.Error("expected non-empty description")
	}
	if texts.URLContent == "" {
		t.Error("expected non-empty urlContent")
	}

	// Mock data should produce no errors (only possible warnings).
	for _, w := range warnings {
		if w.Level == validate.Error {
			t.Errorf("unexpected validation error: %v", w)
		}
	}
}

func TestGenerate_ContextCancellation(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := pg.Generate(ctx, req)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenerate_OnlyTextProperties(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Properties: []generate.PropertySpec{
			{ID: 1, Name: "Material", PropertyType: "text"},
			{ID: 2, Name: "Finish", PropertyType: "text"},
		},
	}

	result, err := pg.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// No non-text properties, so Properties should be nil.
	if result.Properties != nil {
		t.Error("expected nil Properties when only text property specs provided")
	}
	// Text properties should be present per language.
	if result.PropertyTexts == nil {
		t.Fatal("expected non-nil PropertyTexts")
	}
	if len(result.PropertyTexts) != len(generate.SupportedLanguages) {
		t.Errorf("expected PropertyTexts for %d languages, got %d", len(generate.SupportedLanguages), len(result.PropertyTexts))
	}
}

func TestGenerate_OnlyNonTextProperties(t *testing.T) {
	pg := newTestGenerator()
	req := product.GenerationRequest{
		ProductType: "electronics",
		ProductName: "Test Headphones",
		Category:    "Audio",
		Properties: []generate.PropertySpec{
			{ID: 1, Name: "Weight", PropertyType: "int"},
			{ID: 2, Name: "Price", PropertyType: "float"},
		},
	}

	result, err := pg.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// No text properties, so PropertyTexts should be nil.
	if result.PropertyTexts != nil {
		t.Error("expected nil PropertyTexts when only non-text property specs provided")
	}
	// Non-text properties should be present.
	if result.Properties == nil {
		t.Fatal("expected non-nil Properties")
	}
	if len(result.Properties.Values) != 2 {
		t.Errorf("expected 2 non-text property values, got %d", len(result.Properties.Values))
	}
}
