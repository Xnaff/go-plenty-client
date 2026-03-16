package validate

import (
	"strings"
	"testing"

	"github.com/janemig/plentyone/internal/generate"
)

// validProductTexts returns a well-formed ProductTexts that passes validation cleanly.
func validProductTexts() *generate.ProductTexts {
	return &generate.ProductTexts{
		Name:             "Premium Wireless Bluetooth Headphones",
		ShortDescription: "Experience crystal-clear audio with these premium wireless headphones featuring active noise cancellation.",
		Description:      "<p>These premium wireless headphones deliver outstanding audio quality with deep bass and crystal-clear highs. Features include active noise cancellation, 30-hour battery life, and ultra-comfortable ear cups for all-day wear.</p><ul><li>Active noise cancellation</li><li>30-hour battery life</li><li>Bluetooth 5.3</li></ul>",
		TechnicalData:    "<table><tr><th>Weight</th><td>250g</td></tr><tr><th>Driver Size</th><td>40mm</td></tr></table>",
		MetaDescription:  "Buy premium wireless Bluetooth headphones with active noise cancellation and 30-hour battery life online.",
		URLContent:       "premium-wireless-bluetooth-headphones",
		PreviewText:      "Premium wireless headphones with active noise cancellation.",
	}
}

func TestValidateProductTexts_CleanInput(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()

	result, errs := v.ValidateProductTexts(texts, "en")
	if HasErrors(errs) {
		t.Fatalf("expected no errors for valid input, got: %v", errs)
	}
	if result.Name != texts.Name {
		t.Errorf("name changed: got %q, want %q", result.Name, texts.Name)
	}
}

func TestValidateProductTexts_HTMLStrippedInPlainText(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.Name = "The <b>Bold</b> Product"

	result, errs := v.ValidateProductTexts(texts, "en")
	if result.Name != "The Bold Product" {
		t.Errorf("expected HTML stripped from name, got %q", result.Name)
	}
	found := false
	for _, e := range errs {
		if e.Field == "name" && e.Code == "html_stripped" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected html_stripped warning for name field")
	}
}

func TestValidateProductTexts_HTMLPreservedInDescription(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.Description = "<p>This is a paragraph with <strong>emphasis</strong> and a list.</p><ul><li>Item one</li><li>Item two</li></ul>"

	result, errs := v.ValidateProductTexts(texts, "en")
	if !strings.Contains(result.Description, "<p>") {
		t.Error("expected <p> tags preserved in description")
	}
	if !strings.Contains(result.Description, "<strong>") {
		t.Error("expected <strong> tags preserved in description")
	}
	if !strings.Contains(result.Description, "<ul>") {
		t.Error("expected <ul> tags preserved in description")
	}
	// No errors expected for valid HTML in description.
	if HasErrors(errs) {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateProductTexts_UnsafeHTMLRemovedInDescription(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.Description = `<p>Safe text here.</p><script>alert("xss")</script><p>More safe text with enough content to pass minimum length validation checks.</p>`

	result, _ := v.ValidateProductTexts(texts, "en")
	if strings.Contains(result.Description, "<script>") {
		t.Error("expected <script> tags removed from description")
	}
	if !strings.Contains(result.Description, "<p>") {
		t.Error("expected <p> tags preserved in description")
	}
}

func TestValidateProductTexts_FieldLengthTruncation(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	// Name max is 240 chars; create a name that exceeds it.
	texts.Name = strings.Repeat("a", 300)

	result, errs := v.ValidateProductTexts(texts, "en")
	if len([]rune(result.Name)) != 240 {
		t.Errorf("expected name truncated to 240 runes, got %d", len([]rune(result.Name)))
	}
	found := false
	for _, e := range errs {
		if e.Field == "name" && e.Code == "truncated" && e.Level == Warning {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected truncated warning for name field")
	}
}

func TestValidateProductTexts_MinLengthError(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.Name = "ab" // min is 3

	_, errs := v.ValidateProductTexts(texts, "en")
	found := false
	for _, e := range errs {
		if e.Field == "name" && e.Code == "too_short" && e.Level == Error {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected too_short error for name with 2 characters")
	}
}

func TestValidateProductTexts_RequiredFieldEmpty(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.Name = ""

	_, errs := v.ValidateProductTexts(texts, "en")
	found := false
	for _, e := range errs {
		if e.Field == "name" && e.Code == "required" && e.Level == Error {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected required error for empty name field")
	}
}

func TestValidateProductTexts_NonRequiredFieldEmpty(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.ShortDescription = "" // not required

	_, errs := v.ValidateProductTexts(texts, "en")
	for _, e := range errs {
		if e.Field == "shortDescription" && e.Level == Error {
			t.Errorf("did not expect error for empty non-required field, got: %v", e)
		}
	}
}

func TestValidateProductTexts_URLSlugSanitization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Product Name!", "my-product-name"},
		{"a  --b", "a-b"},
		{"Hello_World", "hello-world"},
		{"---leading-trailing---", "leading-trailing"},
		{"UPPERCASE", "uppercase"},
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			texts := validProductTexts()
			texts.URLContent = tt.input

			result, _ := v.ValidateProductTexts(texts, "en")
			if result.URLContent != tt.want {
				t.Errorf("URLContent: got %q, want %q", result.URLContent, tt.want)
			}
		})
	}
}

func TestValidateProductTexts_WrongLanguageWarning(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	// Set a German text in an English field. Must be long enough for detection.
	texts.Name = "Dieses hochwertige Produkt bietet erstklassige Qualitaet und Zuverlaessigkeit fuer anspruchsvolle Kunden"

	_, errs := v.ValidateProductTexts(texts, "en")
	found := false
	for _, e := range errs {
		if e.Field == "name" && e.Code == "wrong_language" && e.Level == Warning {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected wrong_language warning for German text in English field")
	}
}

func TestValidateProductTexts_ShortTextSkipsLanguageDetection(t *testing.T) {
	v := NewValidator()
	texts := validProductTexts()
	texts.PreviewText = "OK Product" // short text, should skip language detection

	_, errs := v.ValidateProductTexts(texts, "de")
	for _, e := range errs {
		if e.Field == "previewText" && e.Code == "wrong_language" {
			t.Error("expected no wrong_language warning for short text")
		}
	}
}

func TestValidateProductTexts_DoesNotMutateOriginal(t *testing.T) {
	v := NewValidator()
	texts := &generate.ProductTexts{
		Name:             "The <b>Bold</b> Product Name Here",
		ShortDescription: "A short description of the product for testing purposes.",
		Description:      "<p>A detailed description of the product with enough text to pass the minimum length check easily.</p>",
		TechnicalData:    "",
		MetaDescription:  "Meta description for SEO that is long enough to pass the minimum length validation.",
		URLContent:       "test-product",
		PreviewText:      "A preview of the product.",
	}
	originalName := texts.Name

	result, _ := v.ValidateProductTexts(texts, "en")
	if texts.Name != originalName {
		t.Error("original texts were mutated")
	}
	if result.Name == originalName {
		t.Error("expected result to be sanitized (different from original)")
	}
}

// Table-driven tests for field constraint checks.
func TestFieldConstraints(t *testing.T) {
	fc := DefaultFieldConstraints()

	tests := []struct {
		field         string
		wantMaxLength int
		wantMinLength int
		wantPlainText bool
		wantRequired  bool
	}{
		{"name", 240, 3, true, true},
		{"shortDescription", 500, 10, true, false},
		{"description", 0, 50, false, true},
		{"technicalData", 0, 0, false, false},
		{"metaDescription", 255, 50, true, true},
		{"urlContent", 240, 3, true, true},
		{"previewText", 200, 5, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			c, ok := fc.Get(tt.field)
			if !ok {
				t.Fatalf("constraint not found for field %q", tt.field)
			}
			if c.MaxLength != tt.wantMaxLength {
				t.Errorf("MaxLength: got %d, want %d", c.MaxLength, tt.wantMaxLength)
			}
			if c.MinLength != tt.wantMinLength {
				t.Errorf("MinLength: got %d, want %d", c.MinLength, tt.wantMinLength)
			}
			if c.PlainText != tt.wantPlainText {
				t.Errorf("PlainText: got %v, want %v", c.PlainText, tt.wantPlainText)
			}
			if c.Required != tt.wantRequired {
				t.Errorf("Required: got %v, want %v", c.Required, tt.wantRequired)
			}
		})
	}
}

func TestValidatePropertyValues_IntMissingValue(t *testing.T) {
	v := NewValidator()
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, IntValue: nil},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Weight", PropertyType: "int"},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if !HasErrors(errs) {
		t.Error("expected error for int property with nil IntValue")
	}
	if errs[0].Code != "missing_value" {
		t.Errorf("expected code missing_value, got %q", errs[0].Code)
	}
}

func TestValidatePropertyValues_FloatMissingValue(t *testing.T) {
	v := NewValidator()
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, FloatValue: nil},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Price", PropertyType: "float"},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if !HasErrors(errs) {
		t.Error("expected error for float property with nil FloatValue")
	}
}

func TestValidatePropertyValues_SelectionInvalidValue(t *testing.T) {
	v := NewValidator()
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, SelectionValue: "purple"},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Color", PropertyType: "selection", Options: []string{"red", "blue", "green"}},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if !HasErrors(errs) {
		t.Error("expected error for selection property with invalid value")
	}
	if errs[0].Code != "invalid_selection" {
		t.Errorf("expected code invalid_selection, got %q", errs[0].Code)
	}
}

func TestValidatePropertyValues_SelectionValidValue(t *testing.T) {
	v := NewValidator()
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, SelectionValue: "blue"},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Color", PropertyType: "selection", Options: []string{"red", "blue", "green"}},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if HasErrors(errs) {
		t.Errorf("expected no errors for valid selection, got: %v", errs)
	}
}

func TestValidatePropertyValues_TextEmptyValue(t *testing.T) {
	v := NewValidator()
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, TextValue: ""},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Description", PropertyType: "text"},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if !HasErrors(errs) {
		t.Error("expected error for text property with empty value")
	}
}

func TestValidatePropertyValues_ValidValues(t *testing.T) {
	v := NewValidator()
	intVal := int64(42)
	floatVal := 9.99
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, IntValue: &intVal},
			{PropertyID: 2, FloatValue: &floatVal},
			{PropertyID: 3, SelectionValue: "red"},
			{PropertyID: 4, TextValue: "Some text value"},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Weight", PropertyType: "int"},
		{ID: 2, Name: "Price", PropertyType: "float"},
		{ID: 3, Name: "Color", PropertyType: "selection", Options: []string{"red", "blue"}},
		{ID: 4, Name: "Material", PropertyType: "text"},
	}

	_, errs := v.ValidatePropertyValues(values, specs)
	if HasErrors(errs) {
		t.Errorf("expected no errors for valid property values, got: %v", errs)
	}
}

func TestValidatePropertyValues_DoesNotMutateOriginal(t *testing.T) {
	v := NewValidator()
	intVal := int64(42)
	values := &generate.PropertyValues{
		Values: []generate.PropertyValue{
			{PropertyID: 1, IntValue: &intVal},
		},
	}
	specs := []generate.PropertySpec{
		{ID: 1, Name: "Weight", PropertyType: "int"},
	}

	result, _ := v.ValidatePropertyValues(values, specs)
	// Modifying the result should not affect the original.
	newVal := int64(99)
	result.Values[0].IntValue = &newVal
	if *values.Values[0].IntValue != 42 {
		t.Error("original property values were mutated")
	}
}

func TestHasErrors(t *testing.T) {
	tests := []struct {
		name string
		errs []ValidationError
		want bool
	}{
		{"nil", nil, false},
		{"empty", []ValidationError{}, false},
		{"warning only", []ValidationError{{Level: Warning}}, false},
		{"error only", []ValidationError{{Level: Error}}, true},
		{"mixed", []ValidationError{{Level: Warning}, {Level: Error}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasErrors(tt.errs); got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorsOnly(t *testing.T) {
	errs := []ValidationError{
		{Level: Warning, Code: "w1"},
		{Level: Error, Code: "e1"},
		{Level: Warning, Code: "w2"},
		{Level: Error, Code: "e2"},
	}
	result := ErrorsOnly(errs)
	if len(result) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(result))
	}
	if result[0].Code != "e1" || result[1].Code != "e2" {
		t.Error("unexpected error codes in result")
	}
}

func TestWarningsOnly(t *testing.T) {
	errs := []ValidationError{
		{Level: Warning, Code: "w1"},
		{Level: Error, Code: "e1"},
		{Level: Warning, Code: "w2"},
	}
	result := WarningsOnly(errs)
	if len(result) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(result))
	}
	if result[0].Code != "w1" || result[1].Code != "w2" {
		t.Error("unexpected warning codes in result")
	}
}

func TestIsCJKLanguage(t *testing.T) {
	tests := []struct {
		lang string
		want bool
	}{
		{"zh", true},
		{"ja", true},
		{"ko", true},
		{"en", false},
		{"de", false},
		{"fr", false},
		{"es", false},
		{"it", false},
		{"pt", false},
		{"ru", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			if got := isCJKLanguage(tt.lang); got != tt.want {
				t.Errorf("isCJKLanguage(%q) = %v, want %v", tt.lang, got, tt.want)
			}
		})
	}
}

func TestEffectiveMinLength(t *testing.T) {
	tests := []struct {
		name    string
		minLen  int
		lang    string
		want    int
	}{
		{"english_50", 50, "en", 50},
		{"chinese_50", 50, "zh", 25},
		{"japanese_50", 50, "ja", 25},
		{"korean_50", 50, "ko", 25},
		{"chinese_10", 10, "zh", 5},
		{"chinese_3", 3, "zh", 1},
		{"chinese_1", 1, "zh", 1},
		{"chinese_0", 0, "zh", 0},
		{"english_0", 0, "en", 0},
		{"german_50", 50, "de", 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMinLength(tt.minLen, tt.lang); got != tt.want {
				t.Errorf("effectiveMinLength(%d, %q) = %d, want %d", tt.minLen, tt.lang, got, tt.want)
			}
		})
	}
}

func TestValidateProductTexts_CJKMinLengthScaling(t *testing.T) {
	v := NewValidator()

	// Chinese metaDescription with 30 characters (above CJK threshold of 25, below Latin threshold of 50).
	chineseTexts := &generate.ProductTexts{
		Name:             "高级无线蓝牙耳机专业版",                                                 // 11 chars, min=1 (3/2)
		ShortDescription: "这款高级无线耳机具有主动降噪功能",                                          // 14 chars, min=5 (10/2)
		Description:      "<p>这款高级无线耳机采用先进技术，提供卓越音质和主动降噪功能。三十小时续航，舒适佩戴。</p>", // well above min
		MetaDescription:  "购买最佳高级无线蓝牙耳机，主动降噪，三十小时续航，免费送货，高品质保证。",               // 31 chars, min=25 (50/2) — passes
		URLContent:       "premium-wireless-headphones-zh",
		PreviewText:      "发现我们的高级无线耳机产品。",
	}

	_, errs := v.ValidateProductTexts(chineseTexts, "zh")
	for _, e := range errs {
		if e.Code == "too_short" {
			t.Errorf("unexpected too_short error for CJK text: %s (field=%s)", e.Message, e.Field)
		}
	}

	// Same text would fail with English language code (50 char minimum applies).
	_, errs = v.ValidateProductTexts(chineseTexts, "en")
	foundMetaShort := false
	for _, e := range errs {
		if e.Field == "metaDescription" && e.Code == "too_short" {
			foundMetaShort = true
		}
	}
	if !foundMetaShort {
		t.Error("expected too_short error for Chinese metaDescription when validated as English (50 char minimum)")
	}
}

func TestValidateProductTexts_CJKStillRejectsTooShort(t *testing.T) {
	v := NewValidator()

	// Even with CJK scaling, very short text should still fail.
	texts := validProductTexts()
	texts.MetaDescription = "短描述" // 3 chars, below even CJK min of 25

	_, errs := v.ValidateProductTexts(texts, "zh")
	found := false
	for _, e := range errs {
		if e.Field == "metaDescription" && e.Code == "too_short" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected too_short error for very short CJK metaDescription (3 chars, min 25)")
	}
}

func TestSanitizeURLSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Product Name!", "my-product-name"},
		{"a  --b", "a-b"},
		{"Hello_World", "hello-world"},
		{"---leading-trailing---", "leading-trailing"},
		{"UPPERCASE", "uppercase"},
		{"simple", "simple"},
		{"with spaces", "with-spaces"},
		{"special@chars#here", "specialcharshere"},
		{"multiple---hyphens", "multiple-hyphens"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := SanitizeURLSlug(tt.input); got != tt.want {
				t.Errorf("SanitizeURLSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello   world", "hello world"},
		{"  leading  ", "leading"},
		{"tabs\t\there", "tabs here"},
		{"new\n\nlines", "new lines"},
		{"mixed \t\n spaces", "mixed spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeWhitespace(tt.input); got != tt.want {
				t.Errorf("NormalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
