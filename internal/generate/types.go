package generate

// SupportedLanguages lists the language codes supported for text generation.
var SupportedLanguages = []string{"en", "de", "es", "fr", "it"}

// ProductTextRequest describes what text to generate for a product.
type ProductTextRequest struct {
	ProductType string   // e.g., "electronics", "food", "fashion"
	ProductName string   // Base product name (may be empty for full generation)
	Category    string   // Category context for relevance
	Language    string   // Target language code: "en", "de", "es", "fr", "it"
	Keywords    []string // Optional SEO keywords to incorporate
	Niche       string   // Niche context for tone/style
}

// ProductTexts is the structured output from text generation.
// JSON tags match PlentyONE CreateDescriptionRequest field names.
type ProductTexts struct {
	Name             string `json:"name"`
	ShortDescription string `json:"shortDescription"`
	Description      string `json:"description"`
	TechnicalData    string `json:"technicalData"`
	MetaDescription  string `json:"metaDescription"`
	URLContent       string `json:"urlContent"`
	PreviewText      string `json:"previewText"`
}

// PropertyValueRequest describes what property values to generate.
type PropertyValueRequest struct {
	ProductType string         // e.g., "electronics", "food", "fashion"
	ProductName string         // Product name for context
	Properties  []PropertySpec // Properties that need values
	Language    string         // For text-type properties
}

// PropertySpec describes a property that needs a value.
type PropertySpec struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	PropertyType string   `json:"propertyType"` // "text", "int", "float", "selection"
	Options      []string `json:"options"`       // For selection type: allowed values
}

// PropertyValues is the structured output from property value generation.
type PropertyValues struct {
	Values []PropertyValue `json:"values"`
}

// PropertyValue is a single generated property value.
type PropertyValue struct {
	PropertyID     int64    `json:"propertyId"`
	TextValue      string   `json:"textValue,omitempty"`
	IntValue       *int64   `json:"intValue,omitempty"`
	FloatValue     *float64 `json:"floatValue,omitempty"`
	SelectionValue string   `json:"selectionValue,omitempty"`
}
