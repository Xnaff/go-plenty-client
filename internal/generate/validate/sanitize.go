package validate

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer provides HTML sanitization with two policies:
// strict (strip all) and permissive (allow safe HTML tags for description fields).
type Sanitizer struct {
	strict      *bluemonday.Policy
	permissive  *bluemonday.Policy
}

// NewSanitizer creates a Sanitizer with both strict and permissive policies.
func NewSanitizer() *Sanitizer {
	strict := bluemonday.StrictPolicy()

	permissive := bluemonday.NewPolicy()
	permissive.AllowElements(
		"p", "ul", "li", "ol", "strong", "em", "br",
		"table", "tr", "td", "th",
		"h2", "h3",
	)

	return &Sanitizer{
		strict:     strict,
		permissive: permissive,
	}
}

// StripAll removes ALL HTML tags and trims whitespace.
func (s *Sanitizer) StripAll(text string) string {
	return strings.TrimSpace(s.strict.Sanitize(text))
}

// SanitizeHTML removes unsafe HTML tags while keeping allowed ones, then trims.
func (s *Sanitizer) SanitizeHTML(text string) string {
	return strings.TrimSpace(s.permissive.Sanitize(text))
}

var (
	slugInvalidChars    = regexp.MustCompile(`[^a-z0-9-]`)
	slugMultipleHyphens = regexp.MustCompile(`-{2,}`)
)

// NormalizeWhitespace collapses multiple spaces and newlines into single spaces, then trims.
func NormalizeWhitespace(text string) string {
	// Replace all whitespace sequences (spaces, tabs, newlines) with a single space.
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(text, " "))
}

// SanitizeURLSlug converts text into a URL-safe slug:
// lowercase, spaces/underscores to hyphens, strip non-alphanumeric/hyphen chars,
// collapse multiple hyphens, trim leading/trailing hyphens.
func SanitizeURLSlug(slug string) string {
	// Lowercase
	slug = strings.ToLower(slug)
	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	// Strip all characters except lowercase a-z, 0-9, hyphens
	slug = slugInvalidChars.ReplaceAllString(slug, "")
	// Collapse multiple hyphens
	slug = slugMultipleHyphens.ReplaceAllString(slug, "-")
	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	return slug
}
