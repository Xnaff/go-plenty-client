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
// Includes selling price, RRP, B2B prices, physical attributes, dimensions, and optional graduated pricing.
type PriceResult struct {
	Price           float64          `json:"price"`
	Currency        string           `json:"currency"`
	RRP             float64          `json:"rrp,omitempty"`
	B2BPrice        float64          `json:"b2bPrice,omitempty"`        // B2B wholesale selling price
	B2BRRP          float64          `json:"b2bRrp,omitempty"`          // B2B recommended retail price
	Model           string           `json:"model,omitempty"`           // Product model identifier
	Weight          float64          `json:"weight,omitempty"`
	WeightUnit      string           `json:"weightUnit,omitempty"`
	SalesUnit       string           `json:"salesUnit,omitempty"`
	LengthCM        int              `json:"lengthCM,omitempty"`
	WidthCM         int              `json:"widthCM,omitempty"`
	HeightCM        int              `json:"heightCM,omitempty"`
	GraduatedPrices []GraduatedPrice `json:"graduatedPrices,omitempty"`
}

// GraduatedPrice represents a quantity-based price tier for bulk pricing.
type GraduatedPrice struct {
	MinimumQuantity int     `json:"minimumQuantity"`
	Price           float64 `json:"price"`
	RRP             float64 `json:"rrp,omitempty"` // RRP at this graduated tier
}

// BatchRequest describes a batch product generation request.
// All products share the same type/niche/category, but each gets unique
// names, prices, and texts in all requested languages.
type BatchRequest struct {
	ProductType string   // e.g., "electronics", "food", "fashion"
	Category    string   // Category context for relevance
	Niche       string   // Niche context for tone/style
	Count       int      // Number of products to generate
	Languages   []string // Target languages (e.g., ["en","de","es","fr","it"])
	Currency    string   // Target currency (default "EUR")
}

// BatchProductResult is the structured output from batch generation.
type BatchProductResult struct {
	CategoryName         map[string]string          `json:"categoryName"`         // Translated parent category name per language
	CategoryDescriptions map[string]string          `json:"categoryDescriptions"` // Translated parent category descriptions per language
	Subcategories        []BatchSubcategory         `json:"subcategories"`        // 2-4 subcategories within the parent category
	Properties           []BatchPropertyDefinition  `json:"properties"`           // Shared property definitions for the batch
	Attributes           []BatchAttributeDefinition `json:"attributes"`           // Shared attribute definitions for variations (e.g., Size, Color)
	Products             []BatchProduct             `json:"products"`
}

// BatchSubcategory defines a subcategory generated within the parent category.
type BatchSubcategory struct {
	Name         string            `json:"name"`         // English subcategory name
	Translations map[string]string `json:"translations"` // lang → translated subcategory name
	Descriptions map[string]string `json:"descriptions"` // lang → translated subcategory description
}

// BatchPropertyDefinition defines a shared selection property across the batch.
// These become filterable properties on category listing pages in PlentyONE.
type BatchPropertyDefinition struct {
	Name         string                         `json:"name"`         // Property name in English (identifier)
	Options      []string                       `json:"options"`      // Available selection values in English
	Translations map[string]PropertyTranslation `json:"translations"` // Translations per language (e.g., {"de":{Name:"Marke",Options:["Sony","Samsung"]}})
}

// PropertyTranslation holds the translated property name and option values for one language.
type PropertyTranslation struct {
	Name    string   `json:"name"`    // Translated property name
	Options []string `json:"options"` // Translated option values (same order as BatchPropertyDefinition.Options)
}

// BatchProductProperty is a property value assigned to a specific product.
// The Name must match a BatchPropertyDefinition.Name and the Value must be
// one of that property's Options.
type BatchProductProperty struct {
	Name  string `json:"name"`  // Property name (must match a definition)
	Value string `json:"value"` // Selected value for this product
}

// BatchAttributeDefinition defines a shared variant-generating attribute across the batch.
// Attributes create cartesian product variations (e.g., Size × Color = multiple variations).
type BatchAttributeDefinition struct {
	Name         string                             `json:"name"`         // Attribute name in English (e.g., "Color", "Size")
	Values       []string                           `json:"values"`       // Available values in English (e.g., ["Red","Blue","Black"])
	Translations map[string]AttributeTranslation    `json:"translations"` // Translations per language
}

// AttributeTranslation holds the translated attribute name and value names for one language.
type AttributeTranslation struct {
	Name   string   `json:"name"`   // Translated attribute name
	Values []string `json:"values"` // Translated value names (same order as BatchAttributeDefinition.Values)
}

// BatchProductAttributeAssignment specifies which attribute values a product uses for variations.
type BatchProductAttributeAssignment struct {
	Name   string   `json:"name"`   // Attribute name (must match a BatchAttributeDefinition.Name)
	Values []string `json:"values"` // Selected values for this product (subset of definition values)
}

// BatchProduct is a single product within a batch result.
// It combines multilingual texts with pricing, physical data, and properties.
type BatchProduct struct {
	Texts           []BatchProductTexts    `json:"texts"`
	Subcategory     string                 `json:"subcategory"` // English subcategory name this product belongs to
	Price           float64                `json:"price"`
	Currency        string                 `json:"currency"`
	RRP             float64                `json:"rrp"`
	B2BPrice        float64                `json:"b2bPrice"`
	B2BRRP          float64                `json:"b2bRrp"`
	Model           string                 `json:"model"`
	Weight          float64                `json:"weight"`
	WeightUnit      string                 `json:"weightUnit"`
	SalesUnit       string                 `json:"salesUnit"`
	LengthCM        int                    `json:"lengthCM"`
	WidthCM         int                    `json:"widthCM"`
	HeightCM        int                    `json:"heightCM"`
	GraduatedPrices []GraduatedPrice       `json:"graduatedPrices"`
	Properties      []BatchProductProperty              `json:"properties"`  // This product's property values
	Attributes      []BatchProductAttributeAssignment  `json:"attributes"`  // This product's attribute assignments for variations
}

// BatchProductTexts is ProductTexts plus a language field for batch context.
type BatchProductTexts struct {
	Language         string `json:"language"`
	Name             string `json:"name"`
	ShortDescription string `json:"shortDescription"`
	Description      string `json:"description"`
	TechnicalData    string `json:"technicalData"`
	MetaDescription  string `json:"metaDescription"`
	URLContent       string `json:"urlContent"`
	PreviewText      string `json:"previewText"`
}

// ToProductTexts converts a BatchProductTexts to the standard ProductTexts type.
func (bt *BatchProductTexts) ToProductTexts() *ProductTexts {
	return &ProductTexts{
		Name:             bt.Name,
		ShortDescription: bt.ShortDescription,
		Description:      bt.Description,
		TechnicalData:    bt.TechnicalData,
		MetaDescription:  bt.MetaDescription,
		URLContent:       bt.URLContent,
		PreviewText:      bt.PreviewText,
	}
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
