package mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time checks: Provider implements Generator, ImageGenerator, and BatchGenerator.
var _ generate.Generator = (*Provider)(nil)
var _ generate.ImageGenerator = (*Provider)(nil)
var _ generate.BatchGenerator = (*Provider)(nil)

// mockPNG1x1 is a 1x1 pixel transparent PNG encoded as base64.
// Used for deterministic test output without network calls.
const mockPNG1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// mockProduct holds diverse product data for batch generation.
type mockProduct struct {
	Name       string
	Model      string
	Weight     float64
	WeightUnit string
	SalesUnit  string
	LengthCM   int
	WidthCM    int
	HeightCM   int
}

// productNamePool provides diverse, realistic product entries for mock batch
// generation. Each product has unique characteristics to produce different
// enrichment and stock photo search results.
var productNamePool = []mockProduct{
	{"Premium Bamboo Wireless Speaker", "BS-400-PRO", 0.85, "g", "piece", 22, 12, 12},
	{"Organic Cotton Travel Blanket", "TB-220-ECO", 1.2, "kg", "piece", 35, 25, 8},
	{"Titanium Camping Cookware Set", "CK-310-TI", 0.45, "g", "set", 20, 20, 15},
	{"Smart LED Desk Lamp", "DL-100-RGB", 1.8, "kg", "piece", 45, 18, 18},
	{"Handcrafted Ceramic Coffee Mug", "CM-050-ART", 0.38, "g", "piece", 12, 10, 10},
	{"Ultra-Light Carbon Fiber Umbrella", "UB-200-CF", 0.29, "g", "piece", 30, 5, 5},
	{"Natural Beeswax Candle Set", "BC-120-NAT", 0.6, "g", "set", 15, 15, 10},
	{"Stainless Steel Water Bottle", "WB-750-SS", 0.32, "g", "piece", 27, 8, 8},
	{"Merino Wool Running Socks", "RS-300-MW", 0.085, "g", "pair", 25, 15, 3},
	{"Portable Solar Phone Charger", "SC-500-SOL", 0.28, "g", "piece", 16, 8, 2},
	{"Artisan Dark Chocolate Bar", "DC-080-ART", 0.1, "g", "piece", 18, 8, 2},
	{"Professional Chef Knife", "KN-200-PRO", 0.23, "g", "piece", 35, 5, 3},
	{"Recycled Ocean Plastic Sunglasses", "SG-100-RPL", 0.032, "g", "piece", 16, 6, 4},
	{"Ergonomic Memory Foam Pillow", "PL-400-MEM", 1.5, "kg", "piece", 60, 40, 14},
	{"Vintage Leather Journal", "LJ-150-VIN", 0.35, "g", "piece", 22, 15, 3},
	{"Wireless Noise-Canceling Earbuds", "EB-300-ANC", 0.058, "g", "pair", 10, 8, 4},
	{"Bamboo Cutting Board", "CB-250-BAM", 0.95, "g", "piece", 40, 30, 3},
	{"Insulated Lunch Bag", "LB-150-INS", 0.42, "g", "piece", 28, 20, 15},
	{"Aromatherapy Essential Oil Set", "EO-100-SET", 0.34, "g", "set", 15, 12, 8},
	{"Foldable Yoga Mat", "YM-500-FLD", 1.1, "kg", "piece", 180, 60, 1},
}

// Provider is a mock Generator that returns deterministic, canned product data.
// It makes zero network calls and is intended for development and testing.
type Provider struct{}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "mock" }

// GenerateProductTexts returns deterministic product texts based on the request.
func (p *Provider) GenerateProductTexts(_ context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, error) {
	return &generate.ProductTexts{
		Name:             fmt.Sprintf("Test Product %s (%s)", req.ProductType, req.Language),
		ShortDescription: fmt.Sprintf("A high-quality %s product for testing purposes.", req.ProductType),
		Description:      fmt.Sprintf("<p>This is a detailed description of a %s product. It includes all the features and benefits that customers want to know about.</p><ul><li>Premium quality materials</li><li>Designed for everyday use</li></ul>", req.ProductType),
		TechnicalData:    "<table><tr><th>Material</th><td>Premium</td></tr><tr><th>Weight</th><td>500g</td></tr><tr><th>Dimensions</th><td>20x15x10cm</td></tr></table>",
		MetaDescription:  fmt.Sprintf("Buy the best %s online. Free shipping. Great quality.", req.ProductType),
		URLContent:       fmt.Sprintf("test-product-%s-%s", req.ProductType, req.Language),
		PreviewText:      fmt.Sprintf("Discover our amazing %s product.", req.ProductType),
	}, nil
}

// GeneratePropertyValues returns type-appropriate deterministic values for each property.
func (p *Provider) GeneratePropertyValues(_ context.Context, req generate.PropertyValueRequest) (*generate.PropertyValues, error) {
	values := make([]generate.PropertyValue, len(req.Properties))
	for i, prop := range req.Properties {
		values[i] = generate.PropertyValue{PropertyID: prop.ID}
		switch prop.PropertyType {
		case "text":
			values[i].TextValue = fmt.Sprintf("Mock value for %s", prop.Name)
		case "int":
			v := int64(42)
			values[i].IntValue = &v
		case "float":
			v := 9.99
			values[i].FloatValue = &v
		case "selection":
			if len(prop.Options) > 0 {
				values[i].SelectionValue = prop.Options[0]
			}
		}
	}
	return &generate.PropertyValues{Values: values}, nil
}

// GeneratePrice returns a deterministic price for testing.
func (p *Provider) GeneratePrice(_ context.Context, req generate.PriceRequest) (*generate.PriceResult, error) {
	currency := req.Currency
	if currency == "" {
		currency = "EUR"
	}
	return &generate.PriceResult{
		Price:      29.99,
		Currency:   currency,
		RRP:        39.99,
		B2BPrice:   19.99,
		B2BRRP:     24.99,
		Model:      "PRO-500",
		Weight:     0.5,
		WeightUnit: "kg",
		SalesUnit:  "piece",
		LengthCM:   30,
		WidthCM:    20,
		HeightCM:   15,
		GraduatedPrices: []generate.GraduatedPrice{
			{MinimumQuantity: 5, Price: 27.99, RRP: 35.99},
			{MinimumQuantity: 10, Price: 24.99, RRP: 32.99},
		},
	}, nil
}

// mockPropertyDefs provides diverse, realistic property definitions for
// mock batch generation. Each product rotates through the options.
var mockPropertyDefs = []generate.BatchPropertyDefinition{
	{
		Name:    "Brand",
		Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"},
		Translations: map[string]generate.PropertyTranslation{
			"de": {Name: "Marke", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
			"es": {Name: "Marca", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
			"fr": {Name: "Marque", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
			"it": {Name: "Marchio", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
			"pt": {Name: "Marca", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
			"ru": {Name: "Бренд", Options: []string{"TechPro", "EcoLine", "UrbanCraft", "PureFit", "NaturePlus"}},
		},
	},
	{
		Name:    "Material",
		Options: []string{"Aluminum", "Bamboo", "Stainless Steel", "Carbon Fiber", "Recycled Plastic"},
		Translations: map[string]generate.PropertyTranslation{
			"de": {Name: "Material", Options: []string{"Aluminium", "Bambus", "Edelstahl", "Kohlefaser", "Recycelter Kunststoff"}},
			"es": {Name: "Material", Options: []string{"Aluminio", "Bambú", "Acero inoxidable", "Fibra de carbono", "Plástico reciclado"}},
			"fr": {Name: "Matériau", Options: []string{"Aluminium", "Bambou", "Acier inoxydable", "Fibre de carbone", "Plastique recyclé"}},
			"it": {Name: "Materiale", Options: []string{"Alluminio", "Bambù", "Acciaio inossidabile", "Fibra di carbonio", "Plastica riciclata"}},
			"pt": {Name: "Material", Options: []string{"Alumínio", "Bambu", "Aço inoxidável", "Fibra de carbono", "Plástico reciclado"}},
			"ru": {Name: "Материал", Options: []string{"Алюминий", "Бамбук", "Нержавеющая сталь", "Углеродное волокно", "Переработанный пластик"}},
		},
	},
	{
		Name:    "Color",
		Options: []string{"Black", "Silver", "White", "Blue", "Green"},
		Translations: map[string]generate.PropertyTranslation{
			"de": {Name: "Farbe", Options: []string{"Schwarz", "Silber", "Weiß", "Blau", "Grün"}},
			"es": {Name: "Color", Options: []string{"Negro", "Plata", "Blanco", "Azul", "Verde"}},
			"fr": {Name: "Couleur", Options: []string{"Noir", "Argent", "Blanc", "Bleu", "Vert"}},
			"it": {Name: "Colore", Options: []string{"Nero", "Argento", "Bianco", "Blu", "Verde"}},
			"pt": {Name: "Cor", Options: []string{"Preto", "Prata", "Branco", "Azul", "Verde"}},
			"ru": {Name: "Цвет", Options: []string{"Чёрный", "Серебристый", "Белый", "Синий", "Зелёный"}},
		},
	},
}

// mockAttributeDefs provides variant-generating attributes for mock batch
// generation. Each product gets all attributes, creating cartesian variations.
var mockAttributeDefs = []generate.BatchAttributeDefinition{
	{
		Name:   "Size",
		Values: []string{"S", "M", "L"},
		Translations: map[string]generate.AttributeTranslation{
			"de": {Name: "Größe", Values: []string{"S", "M", "L"}},
			"es": {Name: "Talla", Values: []string{"S", "M", "L"}},
			"fr": {Name: "Taille", Values: []string{"S", "M", "L"}},
			"it": {Name: "Taglia", Values: []string{"S", "M", "L"}},
		},
	},
	{
		Name:   "Color",
		Values: []string{"Black", "White"},
		Translations: map[string]generate.AttributeTranslation{
			"de": {Name: "Farbe", Values: []string{"Schwarz", "Weiß"}},
			"es": {Name: "Color", Values: []string{"Negro", "Blanco"}},
			"fr": {Name: "Couleur", Values: []string{"Noir", "Blanc"}},
			"it": {Name: "Colore", Values: []string{"Nero", "Bianco"}},
		},
	},
}

// GenerateBatch generates a batch of mock products with varied, unique names
// and characteristics. Each product rotates through the productNamePool to
// ensure diverse enrichment and stock photo search results.
func (p *Provider) GenerateBatch(_ context.Context, req generate.BatchRequest) (*generate.BatchProductResult, error) {
	currency := req.Currency
	if currency == "" {
		currency = "EUR"
	}

	products := make([]generate.BatchProduct, req.Count)

	for i := 0; i < req.Count; i++ {
		pool := productNamePool[i%len(productNamePool)]

		texts := make([]generate.BatchProductTexts, len(req.Languages))
		for j, lang := range req.Languages {
			texts[j] = generate.BatchProductTexts{
				Language:         lang,
				Name:             fmt.Sprintf("%s (%s)", pool.Name, lang),
				ShortDescription: fmt.Sprintf("A high-quality %s for testing purposes.", pool.Name),
				Description:      fmt.Sprintf("<p>This is a detailed description of %s. Premium quality and designed for everyday use.</p><ul><li>Premium quality materials</li><li>Designed for everyday use</li></ul>", pool.Name),
				TechnicalData:    fmt.Sprintf("<table><tr><th>Material</th><td>Premium</td></tr><tr><th>Weight</th><td>%.0fg</td></tr><tr><th>Dimensions</th><td>%dx%dx%dcm</td></tr></table>", pool.Weight*1000, pool.LengthCM, pool.WidthCM, pool.HeightCM),
				MetaDescription:  fmt.Sprintf("Buy the best %s online. Free shipping.", pool.Name),
				URLContent:       fmt.Sprintf("%s-%s", sanitizeSlug(pool.Name), lang),
				PreviewText:      fmt.Sprintf("Discover our amazing %s.", pool.Name),
			}
		}

		basePrice := 29.99 + float64(i)*15.50

		// Assign property values by rotating through options.
		propVals := make([]generate.BatchProductProperty, len(mockPropertyDefs))
		for j, def := range mockPropertyDefs {
			propVals[j] = generate.BatchProductProperty{
				Name:  def.Name,
				Value: def.Options[i%len(def.Options)],
			}
		}

		products[i] = generate.BatchProduct{
			Texts:      texts,
			Price:      basePrice,
			Currency:   currency,
			RRP:        basePrice * 1.25,
			B2BPrice:   basePrice * 0.55,
			B2BRRP:     basePrice * 0.75,
			Model:      pool.Model,
			Weight:     pool.Weight,
			WeightUnit: pool.WeightUnit,
			SalesUnit:  pool.SalesUnit,
			LengthCM:   pool.LengthCM,
			WidthCM:    pool.WidthCM,
			HeightCM:   pool.HeightCM,
			GraduatedPrices: []generate.GraduatedPrice{
				{MinimumQuantity: 5, Price: basePrice * 0.90, RRP: basePrice * 1.15},
				{MinimumQuantity: 10, Price: basePrice * 0.80, RRP: basePrice * 1.05},
			},
			Properties: propVals,
		}
	}

	// Build translated category names from the niche/category.
	catName := req.Niche
	if catName == "" {
		catName = req.Category
	}
	if catName == "" {
		catName = "Products"
	}
	categoryNames := map[string]string{"en": catName}
	mockCatTranslations := map[string]string{
		"de": "Produkte", "es": "Productos", "fr": "Produits",
		"it": "Prodotti", "pt": "Produtos", "ru": "Продукты",
	}
	for _, lang := range req.Languages {
		if lang != "en" {
			if t, ok := mockCatTranslations[lang]; ok {
				categoryNames[lang] = t
			}
		}
	}

	// Category descriptions per language.
	categoryDescriptions := map[string]string{
		"en": fmt.Sprintf("Explore our wide selection of %s. Quality products at great prices.", catName),
		"de": fmt.Sprintf("Entdecken Sie unsere große Auswahl an %s. Qualitätsprodukte zu günstigen Preisen.", catName),
		"es": fmt.Sprintf("Explore nuestra amplia selección de %s. Productos de calidad a excelentes precios.", catName),
		"fr": fmt.Sprintf("Découvrez notre large sélection de %s. Produits de qualité à des prix compétitifs.", catName),
		"it": fmt.Sprintf("Scopri la nostra ampia selezione di %s. Prodotti di qualità a prezzi competitivi.", catName),
	}

	// Mock subcategories.
	mockSubcategories := []generate.BatchSubcategory{
		{
			Name: "Smartphones",
			Translations: map[string]string{
				"de": "Smartphones", "es": "Teléfonos inteligentes",
				"fr": "Smartphones", "it": "Smartphone",
			},
			Descriptions: map[string]string{
				"en": "Discover the latest smartphones with cutting-edge technology.",
				"de": "Entdecken Sie die neuesten Smartphones mit modernster Technologie.",
				"es": "Descubra los últimos teléfonos inteligentes con tecnología de vanguardia.",
				"fr": "Découvrez les derniers smartphones avec une technologie de pointe.",
				"it": "Scopri gli ultimi smartphone con tecnologia all'avanguardia.",
			},
		},
		{
			Name: "Audio Equipment",
			Translations: map[string]string{
				"de": "Audiogeräte", "es": "Equipos de audio",
				"fr": "Équipement audio", "it": "Apparecchiature audio",
			},
			Descriptions: map[string]string{
				"en": "Premium audio equipment for the perfect sound experience.",
				"de": "Premium-Audiogeräte für das perfekte Klangerlebnis.",
				"es": "Equipos de audio premium para la experiencia de sonido perfecta.",
				"fr": "Équipement audio haut de gamme pour une expérience sonore parfaite.",
				"it": "Apparecchiature audio premium per un'esperienza sonora perfetta.",
			},
		},
	}

	// Assign subcategories and attribute values to products.
	for i := range products {
		products[i].Subcategory = mockSubcategories[i%len(mockSubcategories)].Name

		// Assign all attribute definitions as product attribute assignments.
		// Each product gets Size S,M,L × Color Black,White = 6 variations.
		products[i].Attributes = []generate.BatchProductAttributeAssignment{
			{Name: "Size", Values: []string{"S", "M", "L"}},
			{Name: "Color", Values: []string{"Black", "White"}},
		}
	}

	return &generate.BatchProductResult{
		CategoryName:         categoryNames,
		CategoryDescriptions: categoryDescriptions,
		Subcategories:        mockSubcategories,
		Properties:           mockPropertyDefs,
		Attributes:           mockAttributeDefs,
		Products:             products,
	}, nil
}

// GenerateProductImage returns a deterministic 1x1 transparent PNG for testing.
func (p *Provider) GenerateProductImage(_ context.Context, req generate.ImageRequest) (*generate.ImageResult, error) {
	return &generate.ImageResult{
		Base64Data:    mockPNG1x1,
		RevisedPrompt: req.BuildPrompt(),
		Format:        "png",
	}, nil
}

// sanitizeSlug converts a product name to a URL-friendly slug.
func sanitizeSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
