// Package quality provides product quality scoring with configurable rules
// and thresholds. It evaluates text completeness, image availability, and
// data richness to produce a QualityReport indicating whether a generated
// product meets minimum standards.
package quality

import "github.com/janemig/plentyone/internal/generate"

// Thresholds configures the minimum acceptable scores for each quality
// dimension. Products must meet ALL thresholds to pass.
type Thresholds struct {
	MinOverallScore float64
	MinTextScore    float64
	MinImageScore   float64
	MinDataScore    float64
}

// DefaultThresholds returns lenient defaults suitable for initial product
// generation where not all enrichment sources may be available.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MinOverallScore: 0.6,
		MinTextScore:    0.5,
		MinImageScore:   0.4,
		MinDataScore:    0.3,
	}
}

// ScoringInput contains all the data needed to evaluate a product's quality.
type ScoringInput struct {
	ProductName      string
	ProductType      string
	Texts            map[string]*generate.ProductTexts // lang -> texts
	ImageCount       int
	HasAIImage       bool
	HasStockPhoto    bool
	EnrichmentFields map[string]string
	Languages        []string
}

// RuleResult is the outcome of evaluating a single scoring rule.
type RuleResult struct {
	RuleName string  `json:"rule_name"`
	Score    float64 `json:"score"` // 0.0 - 1.0
	Details  string  `json:"details"`
}

// QualityReport is the complete quality assessment for a product.
type QualityReport struct {
	OverallScore float64      `json:"overall_score"`
	TextScore    float64      `json:"text_score"`
	ImageScore   float64      `json:"image_score"`
	DataScore    float64      `json:"data_score"`
	RuleResults  []RuleResult `json:"rule_results"`
	Pass         bool         `json:"pass"`
	Flags        []string     `json:"flags"` // Human-readable issues
}

// Rule is the interface that quality scoring rules must implement.
type Rule interface {
	// Name returns a human-readable identifier for this rule.
	Name() string
	// Evaluate scores a product against this rule.
	Evaluate(input *ScoringInput) RuleResult
}
