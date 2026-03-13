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

// PriceRequest describes what price to generate for a product.
type PriceRequest struct {
	ProductType string // e.g., "electronics", "food", "fashion"
	ProductName string // Product name for context
	Category    string // Category context for pricing
	Niche       string // Niche context for pricing
	Currency    string // Target currency (default "EUR")
}

// PriceResult is the structured output from price generation.
type PriceResult struct {
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
}

// ImageRequest describes what product image to generate.
type ImageRequest struct {
	ProductName string // Base product name
	ProductType string // e.g., "electronics", "food", "fashion"
	Category    string // Category context for relevance
	Style       string // Image style instructions (default: "product photography, white background, studio lighting")
	Size        string // Image dimensions (default: "1024x1024")
	Quality     string // Image quality level (default: "medium")
}

// BuildPrompt constructs a text prompt for AI image generation from the request fields.
func (r ImageRequest) BuildPrompt() string {
	style := r.Style
	if style == "" {
		style = "product photography, white background, studio lighting"
	}
	return "A professional product photo of " + r.ProductName +
		", a " + r.ProductType + " product. " +
		style + ". Clean, commercial e-commerce image suitable for an online store."
}

// ImageResult is the output from AI image generation.
type ImageResult struct {
	Base64Data    string // Base64-encoded image data
	RevisedPrompt string // The prompt as revised/interpreted by the AI model
	Format        string // Image format: "png", "webp", or "jpeg"
}

// imageRequestDefaults returns defaults for unset fields.
func imageRequestDefaults(req *ImageRequest) {
	if req.Style == "" {
		req.Style = "product photography, white background, studio lighting"
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}
	if req.Quality == "" {
		req.Quality = "medium"
	}
}
