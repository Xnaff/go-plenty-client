package quality

// Scorer evaluates product quality against configurable thresholds using a
// set of rules. The default rule set covers text completeness, image
// availability, and data richness.
type Scorer struct {
	thresholds Thresholds
	rules      []Rule
}

// NewScorer creates a Scorer with the three default quality rules.
func NewScorer(thresholds Thresholds) *Scorer {
	return &Scorer{
		thresholds: thresholds,
		rules: []Rule{
			&TextCompletenessRule{},
			&ImageAvailabilityRule{},
			&DataRichnessRule{},
		},
	}
}

// Score evaluates a product against all rules and produces a QualityReport.
// OverallScore is a weighted average: text 0.5, image 0.3, data 0.2.
// Pass requires OverallScore AND each category score to meet its threshold.
func (s *Scorer) Score(input *ScoringInput) *QualityReport {
	report := &QualityReport{
		RuleResults: make([]RuleResult, 0, len(s.rules)),
	}

	// Run each rule.
	scoresByRule := make(map[string]float64, len(s.rules))
	for _, rule := range s.rules {
		result := rule.Evaluate(input)
		report.RuleResults = append(report.RuleResults, result)
		scoresByRule[rule.Name()] = result.Score
	}

	// Map rule scores to categories.
	report.TextScore = scoresByRule["text_completeness"]
	report.ImageScore = scoresByRule["image_availability"]
	report.DataScore = scoresByRule["data_richness"]

	// Weighted average: text matters most for e-commerce.
	report.OverallScore = report.TextScore*0.5 + report.ImageScore*0.3 + report.DataScore*0.2

	// Pass requires meeting ALL thresholds.
	report.Pass = report.OverallScore >= s.thresholds.MinOverallScore &&
		report.TextScore >= s.thresholds.MinTextScore &&
		report.ImageScore >= s.thresholds.MinImageScore &&
		report.DataScore >= s.thresholds.MinDataScore

	// Collect flags from rules that scored below their category threshold.
	if report.TextScore < s.thresholds.MinTextScore {
		for _, rr := range report.RuleResults {
			if rr.RuleName == "text_completeness" {
				report.Flags = append(report.Flags, "text: "+rr.Details)
			}
		}
	}
	if report.ImageScore < s.thresholds.MinImageScore {
		for _, rr := range report.RuleResults {
			if rr.RuleName == "image_availability" {
				report.Flags = append(report.Flags, "image: "+rr.Details)
			}
		}
	}
	if report.DataScore < s.thresholds.MinDataScore {
		for _, rr := range report.RuleResults {
			if rr.RuleName == "data_richness" {
				report.Flags = append(report.Flags, "data: "+rr.Details)
			}
		}
	}

	return report
}
