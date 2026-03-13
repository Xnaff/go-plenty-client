package quality

import "fmt"

// textFields lists the 7 text fields expected per language. These correspond
// to the ProductTexts struct fields.
var textFields = []string{
	"name",
	"shortDescription",
	"description",
	"technicalData",
	"metaDescription",
	"urlContent",
	"previewText",
}

// TextCompletenessRule evaluates how many text fields are filled across all
// configured languages. Score = filledFields / totalExpectedFields.
type TextCompletenessRule struct{}

func (r *TextCompletenessRule) Name() string { return "text_completeness" }

func (r *TextCompletenessRule) Evaluate(input *ScoringInput) RuleResult {
	if len(input.Languages) == 0 || len(input.Texts) == 0 {
		return RuleResult{
			RuleName: r.Name(),
			Score:    0.0,
			Details:  "no text data available",
		}
	}

	totalExpected := len(input.Languages) * len(textFields)
	filled := 0
	var flags []string

	for _, lang := range input.Languages {
		texts, ok := input.Texts[lang]
		if !ok {
			for _, field := range textFields {
				flags = append(flags, fmt.Sprintf("missing %s for %s", field, lang))
			}
			continue
		}

		fieldValues := map[string]string{
			"name":             texts.Name,
			"shortDescription": texts.ShortDescription,
			"description":      texts.Description,
			"technicalData":    texts.TechnicalData,
			"metaDescription":  texts.MetaDescription,
			"urlContent":       texts.URLContent,
			"previewText":      texts.PreviewText,
		}

		for _, field := range textFields {
			if fieldValues[field] != "" {
				filled++
			} else {
				flags = append(flags, fmt.Sprintf("missing %s for %s", field, lang))
			}
		}
	}

	score := float64(filled) / float64(totalExpected)

	details := fmt.Sprintf("%d/%d text fields filled across %d languages", filled, totalExpected, len(input.Languages))
	if len(flags) > 0 {
		details += fmt.Sprintf("; %d missing fields", len(flags))
	}

	return RuleResult{
		RuleName: r.Name(),
		Score:    score,
		Details:  details,
	}
}

// ImageAvailabilityRule evaluates image coverage for a product.
// Score 1.0 if both AI image AND stock photo present,
// Score 0.7 if either one present,
// Score 0.0 if no images at all.
type ImageAvailabilityRule struct{}

func (r *ImageAvailabilityRule) Name() string { return "image_availability" }

func (r *ImageAvailabilityRule) Evaluate(input *ScoringInput) RuleResult {
	var score float64
	var details string

	switch {
	case input.HasAIImage && input.HasStockPhoto:
		score = 1.0
		details = fmt.Sprintf("both AI and stock images available (%d total)", input.ImageCount)
	case input.HasAIImage:
		score = 0.7
		details = fmt.Sprintf("AI-generated image available (%d total)", input.ImageCount)
	case input.HasStockPhoto:
		score = 0.7
		details = fmt.Sprintf("stock photo available (%d total)", input.ImageCount)
	default:
		score = 0.0
		details = "no product images"
	}

	return RuleResult{
		RuleName: r.Name(),
		Score:    score,
		Details:  details,
	}
}

// DataRichnessRule evaluates how much enrichment data is available.
// Score tiers based on number of enrichment fields populated:
// 0 fields = 0.2 (baseline for AI-generated content),
// 1-2 = 0.5, 3-4 = 0.7, 5+ = 1.0.
type DataRichnessRule struct{}

func (r *DataRichnessRule) Name() string { return "data_richness" }

func (r *DataRichnessRule) Evaluate(input *ScoringInput) RuleResult {
	fieldCount := len(input.EnrichmentFields)

	var score float64
	switch {
	case fieldCount >= 5:
		score = 1.0
	case fieldCount >= 3:
		score = 0.7
	case fieldCount >= 1:
		score = 0.5
	default:
		score = 0.2 // Baseline for AI-generated content without enrichment
	}

	details := fmt.Sprintf("%d enrichment fields populated", fieldCount)
	if fieldCount == 0 {
		details = "no enrichment data available"
	}

	return RuleResult{
		RuleName: r.Name(),
		Score:    score,
		Details:  details,
	}
}
