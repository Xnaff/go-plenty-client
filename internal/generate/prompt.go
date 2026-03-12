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
