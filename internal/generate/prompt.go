package generate

import (
	"fmt"
	"strings"
)

// SystemPromptForLanguage returns a system prompt written IN the target language
// to prime the model for native-quality text generation. The prompt itself is
// in the target language (not English instructions to "write in German").
func SystemPromptForLanguage(lang string) string {
	switch lang {
	case "de":
		return `Du bist ein erfahrener E-Commerce-Texter. Schreibe alle Produkttexte auf natürlichem, professionellem Deutsch. Verwende keine maschinelle Übersetzung. Beachte deutsche SEO-Best-Practices, korrekte Grammatik und natürliche Formulierungen. Verwende die formelle Anrede (Sie). Alle Maßeinheiten in metrischem System.`
	case "es":
		return `Eres un redactor experto en comercio electrónico. Escribe todos los textos de producto en español natural y profesional. No uses traducción automática. Aplica las mejores prácticas de SEO en español, gramática correcta y expresiones naturales. Usa el sistema métrico para todas las unidades de medida.`
	case "fr":
		return `Vous êtes un rédacteur e-commerce expérimenté. Rédigez tous les textes produit en français naturel et professionnel. N'utilisez pas de traduction automatique. Appliquez les meilleures pratiques SEO en français, une grammaire correcte et des formulations naturelles. Utilisez le vouvoiement. Système métrique pour les mesures.`
	case "it":
		return `Sei un copywriter e-commerce esperto. Scrivi tutti i testi dei prodotti in italiano naturale e professionale. Non usare traduzioni automatiche. Applica le migliori pratiche SEO in italiano, grammatica corretta e formulazioni naturali. Usa il sistema metrico per tutte le unità di misura.`
	default: // "en"
		return `You are an expert e-commerce copywriter. Write all product texts in natural, professional English. Follow SEO best practices, use correct grammar, and write engaging product descriptions that convert. Use metric measurements where appropriate.`
	}
}

// BuildProductTextPrompt constructs the user prompt for product text generation.
// The system prompt (language-specific) is set separately via SystemPromptForLanguage.
func BuildProductTextPrompt(req ProductTextRequest) string {
	var b strings.Builder

	b.WriteString("Generate complete e-commerce product texts for the following product:\n\n")
	b.WriteString(fmt.Sprintf("Product Type: %s\n", req.ProductType))
	b.WriteString(fmt.Sprintf("Category: %s\n", req.Category))

	if req.ProductName != "" {
		b.WriteString(fmt.Sprintf("Product Name Hint: %s\n", req.ProductName))
	}
	if req.Niche != "" {
		b.WriteString(fmt.Sprintf("Niche: %s\n", req.Niche))
	}
	if len(req.Keywords) > 0 {
		b.WriteString(fmt.Sprintf("SEO Keywords: %s\n", strings.Join(req.Keywords, ", ")))
	}

	b.WriteString(`
Requirements for each field:
- name: A compelling, SEO-friendly product name (max 240 characters, plain text, no HTML)
- shortDescription: A brief, punchy summary (max 500 characters, plain text, no HTML)
- description: A detailed product description with key features and benefits (HTML allowed, use <p>, <ul>, <li>, <strong>, <em> tags for structure)
- technicalData: Technical specifications in structured format (HTML allowed, use table or list format with <table>, <tr>, <td>, <th>, <ul>, <li> tags)
- metaDescription: SEO meta description (max 155 characters for optimal search display, plain text, no HTML)
- urlContent: URL-friendly slug (lowercase, hyphens only, no special characters, no spaces, max 240 characters)
- previewText: A one-line teaser for product listings (max 200 characters, plain text, no HTML)

All text must be written natively in the target language (not translated).
Focus on accuracy, natural language, and e-commerce conversion.`)

	return b.String()
}

// BuildPricePrompt constructs the user prompt for product price generation.
func BuildPricePrompt(req PriceRequest) string {
	var b strings.Builder

	currency := req.Currency
	if currency == "" {
		currency = "EUR"
	}

	b.WriteString("Generate a realistic retail price for the following product:\n\n")
	b.WriteString(fmt.Sprintf("Product Type: %s\n", req.ProductType))
	if req.ProductName != "" {
		b.WriteString(fmt.Sprintf("Product Name: %s\n", req.ProductName))
	}
	if req.Category != "" {
		b.WriteString(fmt.Sprintf("Category: %s\n", req.Category))
	}
	if req.Niche != "" {
		b.WriteString(fmt.Sprintf("Niche: %s\n", req.Niche))
	}
	b.WriteString(fmt.Sprintf("Currency: %s\n", currency))

	b.WriteString(`
Generate ALL of the following for this product:

1. PRICE: A realistic retail selling price for an e-commerce store. Use natural prices (e.g., 29.99, 149.00, 7.50).

2. RRP (Recommended Retail Price): The manufacturer's suggested retail price, typically 10-40% higher than the selling price. This shows customers they're getting a deal.

3. WEIGHT: A realistic product weight as a number. Use metric units.
   - weightUnit: Use "kg" for items over 1kg, "g" for lighter items.

4. SALES UNIT: How this product is sold. Choose one of: "piece", "kg", "g", "liter", "ml", "meter", "pair", "set", "pack", "box".
   Most products use "piece". Use weight/volume units for bulk goods (e.g., coffee sold by "kg", juice by "liter").

5. GRADUATED PRICES (optional): If this product would realistically be sold with quantity discounts (e.g., office supplies, consumables, bulk goods, packaging materials), include graduated price tiers. For products that don't suit quantity pricing (e.g., electronics, furniture, luxury items), leave graduatedPrices as an empty array.
   Each tier needs minimumQuantity (int), price (discounted unit price at that quantity), and rrp (recommended retail price at that quantity tier, typically 15-30% above the tier price).
   Example: base price 10.00, graduated: [{minimumQuantity: 5, price: 9.00, rrp: 11.50}, {minimumQuantity: 10, price: 8.00, rrp: 10.50}]
   Prices must decrease as quantity increases. Use 2-4 tiers maximum.

6. DIMENSIONS: Realistic product or packaging dimensions as whole numbers in centimeters (integers only, no decimals).
   - lengthCM: Length in cm (e.g., 30, 15, 100)
   - widthCM: Width in cm (e.g., 20, 10, 50)
   - heightCM: Height in cm (e.g., 15, 5, 40)
   Use realistic dimensions appropriate for the product type. For small items like jewelry use small values, for furniture use large values.

7. B2B PRICE: A wholesale/B2B selling price, typically 30-50% lower than the retail price. This is the price for business customers buying in bulk.

8. B2B RRP: A B2B recommended retail price, typically 10-20% higher than the B2B selling price.

9. MODEL: A short product model identifier (e.g., "PRO-500", "MT-2024", "XL-100"). Alphanumeric with optional hyphens, uppercase, max 20 characters.`)

	return b.String()
}

// BatchSystemPrompt returns the system prompt for batch product generation.
// Since batch generation cannot use per-language system prompts, a general
// multilingual e-commerce expert prompt is used instead.
func BatchSystemPrompt() string {
	return `You are an expert e-commerce product data generator. You generate complete, realistic product listings with multilingual texts, pricing, and physical specifications. Each product must be unique and distinct with different names, prices, dimensions, and characteristics. Write natively in each target language (not translated). Follow e-commerce SEO best practices.`
}

// BuildBatchPrompt constructs the user prompt for batch product generation.
// It combines text, pricing, and physical data requirements for multiple
// products into a single prompt with embedded per-language style guides.
func BuildBatchPrompt(req BatchRequest) string {
	var b strings.Builder

	currency := req.Currency
	if currency == "" {
		currency = "EUR"
	}

	b.WriteString(fmt.Sprintf("Generate %d UNIQUE and DIVERSE e-commerce products for the following context:\n\n", req.Count))
	b.WriteString(fmt.Sprintf("Product Type: %s\n", req.ProductType))
	b.WriteString(fmt.Sprintf("Category: %s\n", req.Category))
	if req.Niche != "" {
		b.WriteString(fmt.Sprintf("Niche: %s\n", req.Niche))
	}
	b.WriteString(fmt.Sprintf("Currency: %s\n", currency))

	b.WriteString(fmt.Sprintf("\nFor EACH product, generate texts in ALL of these languages: %s\n", strings.Join(req.Languages, ", ")))

	// Embed per-language writing instructions.
	b.WriteString("\n=== LANGUAGE-SPECIFIC WRITING INSTRUCTIONS ===\n")
	for _, lang := range req.Languages {
		b.WriteString(fmt.Sprintf("\n[%s]: %s\n", lang, SystemPromptForLanguage(lang)))
	}

	b.WriteString(`
=== TEXT FIELD REQUIREMENTS ===
For each language, generate these fields:
- name: A compelling, SEO-friendly product name (max 240 characters, plain text, no HTML)
- shortDescription: A brief, punchy summary (max 500 characters, plain text, no HTML)
- description: A detailed product description with key features and benefits (HTML allowed, use <p>, <ul>, <li>, <strong>, <em> tags)
- technicalData: Technical specifications (HTML allowed, use table or list format with <table>, <tr>, <td>, <th>, <ul>, <li> tags)
- metaDescription: SEO meta description (max 155 characters, plain text, no HTML)
- urlContent: URL-friendly slug (lowercase, hyphens only, no special characters, max 240 characters)
- previewText: A one-line teaser for product listings (max 200 characters, plain text, no HTML)

All text must be written NATIVELY in each target language (not translated from another language).
Each product MUST have a UNIQUE, DISTINCT name and different characteristics.

=== PRICING AND PHYSICAL DATA (per product) ===
1. price: Realistic retail selling price (use natural pricing like 29.99, 149.00, 7.50)
2. currency: "` + currency + `"
3. rrp: Recommended retail price (typically 15-30% higher than price)
4. b2bPrice: Wholesale/B2B selling price (typically 30-50% lower than retail price)
5. b2bRrp: B2B recommended retail price (typically 10-20% higher than b2bPrice)
6. model: A realistic product model/SKU identifier (alphanumeric + hyphens, uppercase, max 20 chars, e.g., "PRO-500", "MT-2024")
7. weight: Product weight as a number (use metric units)
8. weightUnit: "kg" for items over 1kg, "g" for lighter items
9. salesUnit: How the product is sold: "piece", "kg", "g", "liter", "ml", "meter", "pair", "set", "pack", "box"
10. lengthCM, widthCM, heightCM: Package dimensions as whole numbers in centimeters (integers only, no decimals)
11. graduatedPrices: 2-3 quantity-based price tiers with minimumQuantity (int), price (discounted unit price), and rrp (RRP at that tier). Leave empty if quantity pricing doesn't suit the product.

Products must be DIVERSE — vary the names, prices, weights, dimensions, and specifications significantly.

=== CATEGORY NAME TRANSLATIONS ===
Translate the category name "` + req.Category + `" into ALL target languages.
Return a "categoryName" object with language codes as keys and the translated category name as values.
Example: {"en": "Electronics", "de": "Elektronik", "es": "Electrónica", "fr": "Électronique"}
Include ALL these languages: ` + strings.Join(req.Languages, ", ") + `

=== CATEGORY DESCRIPTIONS ===
For each language, write a short e-commerce category description (2-3 sentences, plain text, max 500 characters) for the parent category "` + req.Category + `".
This description appears on the category page and should be SEO-friendly and informative.
Return as "categoryDescriptions": {"en": "...", "de": "...", ...}
Include ALL these languages: ` + strings.Join(req.Languages, ", ") + `

=== SUBCATEGORIES ===
Generate 2-4 subcategories within the "` + req.Category + `" category that are relevant to the products being generated.
Each subcategory should represent a distinct product grouping within the parent category.
Each subcategory has:
- "name": English subcategory name
- "translations": object mapping each non-English language code to the translated subcategory name
- "descriptions": object mapping ALL language codes to a short description (1-2 sentences, plain text, max 300 characters) for that subcategory

Return as "subcategories": [{"name": "Smartphones", "translations": {"de": "Smartphones", "es": "Teléfonos inteligentes"}, "descriptions": {"en": "Discover the latest smartphones...", "de": "Entdecken Sie die neuesten Smartphones..."}}, ...]
Include ALL these languages: ` + strings.Join(req.Languages, ", ") + `

Each product MUST specify which subcategory it belongs to via a "subcategory" field containing the English subcategory name from the list above.

=== PROPERTY DEFINITIONS (for category page filters) ===
Generate 2-5 selection-type properties that are appropriate for filtering ` + req.ProductType + ` products on a category listing page.
Properties are SHARED across ALL products in the batch — they go in the top-level "properties" array.
Each property has a "name" (English identifier), "options" (English values), and "translations" with the property name and option values translated to each non-English language.

Good examples for different niches:
- Electronics: Brand, Connectivity, Color, Power Source, Warranty
- Fashion: Brand, Material, Season, Pattern, Care Instructions
- Food: Brand, Dietary, Origin, Packaging, Certification
- Home: Brand, Material, Room, Style, Finish

Rules:
- Generate 2-5 properties with 3-6 options each
- "name" and "options" should be in English (they serve as identifiers)
- "translations" maps each non-English language to an object with "name" (translated property name) and "options" (translated option values in the SAME ORDER as the English options)
- Option values that are proper nouns (brand names, etc.) should stay the same in all languages
- For EACH product, assign a value from the defined English options for EVERY property
- Every product MUST have at least 2 property values in its "properties" array
- Each product's property value MUST be one of the English options defined in the top-level properties

Example property with translations:
{"name": "Color", "options": ["Red", "Blue", "Green"], "translations": {"de": {"name": "Farbe", "options": ["Rot", "Blau", "Grün"]}, "fr": {"name": "Couleur", "options": ["Rouge", "Bleu", "Vert"}}}}

=== ATTRIBUTE DEFINITIONS (for product variations) ===
Attributes define variant-generating dimensions that create separate product variations (e.g., Size × Color = multiple purchasable items).
ONLY generate attributes if the product type naturally benefits from variations:
- Fashion/Clothing: Size + Color (e.g., S/M/L/XL × Red/Blue/Black)
- Electronics: Storage + Color (e.g., 64GB/128GB/256GB × Black/White)
- Shoes: Size + Color
- Furniture: Material + Size (e.g., Oak/Walnut × Small/Large)
If the product type does NOT benefit from variations (e.g., unique items, books, food, tools), return an EMPTY "attributes" array.

Rules:
- Generate 0-3 attributes with 2-5 values each (0 means no variations)
- "name" and "values" in English
- "translations" maps each non-English language to an object with "name" (translated attribute name) and "values" (translated value names in SAME ORDER)
- Values that are universal (S/M/L/XL, dimensions like 64GB) may stay the same across languages
- Each product specifies which attribute values it uses in its "attributes" array
- The cartesian product of all attribute values creates the variations (e.g., 3 sizes × 3 colors = 9 variations)

Top-level format (shared across all products):
"attributes": [{"name": "Size", "values": ["S","M","L","XL"], "translations": {"de": {"name": "Größe", "values": ["S","M","L","XL"]}, "fr": {"name": "Taille", "values": ["S","M","L","XL"]}}}]

Per product format (which values this product uses):
"attributes": [{"name": "Size", "values": ["S","M","L"]}, {"name": "Color", "values": ["Red","Blue"]}]
Products without variations should have: "attributes": []
Include ALL these languages: ` + strings.Join(req.Languages, ", ") + "`")

	return b.String()
}

// BuildPropertyValuePrompt constructs the user prompt for property value generation.
func BuildPropertyValuePrompt(req PropertyValueRequest) string {
	var b strings.Builder

	b.WriteString("Generate property values for the following product:\n\n")
	b.WriteString(fmt.Sprintf("Product Type: %s\n", req.ProductType))
	if req.ProductName != "" {
		b.WriteString(fmt.Sprintf("Product Name: %s\n", req.ProductName))
	}
	b.WriteString(fmt.Sprintf("Language: %s\n", req.Language))
	b.WriteString("\nProperties to fill:\n\n")

	for _, prop := range req.Properties {
		b.WriteString(fmt.Sprintf("- Property ID %d: \"%s\" (type: %s)", prop.ID, prop.Name, prop.PropertyType))
		switch prop.PropertyType {
		case "selection":
			if len(prop.Options) > 0 {
				b.WriteString(fmt.Sprintf(" -- Choose ONLY from: [%s]", strings.Join(prop.Options, ", ")))
			} else {
				b.WriteString(" -- No options available, leave empty")
			}
		case "int":
			b.WriteString(" -- Provide a numeric integer value only")
		case "float":
			b.WriteString(" -- Provide a numeric decimal value only")
		case "text":
			b.WriteString(fmt.Sprintf(" -- Provide a text value in %s", req.Language))
		}
		b.WriteString("\n")
	}

	b.WriteString("\nReturn a value for each property that is realistic and appropriate for this product type.")

	return b.String()
}
