package validate

import (
	"fmt"
	"unicode/utf8"

	"github.com/janemig/plentyone/internal/generate"
)

// Validator validates and sanitizes AI-generated product data.
type Validator struct {
	sanitizer    *Sanitizer
	langDetector *LanguageDetector
	fields       FieldConstraints
}

// NewValidator creates a Validator with all sub-components initialized.
func NewValidator() *Validator {
	return &Validator{
		sanitizer:    NewSanitizer(),
		langDetector: NewLanguageDetector(),
		fields:       DefaultFieldConstraints(),
	}
}

// ValidateProductTexts validates and sanitizes all 7 text fields.
// Returns a COPY of the texts (does not mutate the original) and any validation errors.
func (v *Validator) ValidateProductTexts(texts *generate.ProductTexts, lang string) (*generate.ProductTexts, []ValidationError) {
	// Create a copy to avoid mutating the original.
	result := *texts
	var errs []ValidationError

	// Helper to get and set field values by name.
	type fieldAccessor struct {
		name string
		get  func() string
		set  func(string)
	}

	accessors := []fieldAccessor{
		{"name", func() string { return result.Name }, func(s string) { result.Name = s }},
		{"shortDescription", func() string { return result.ShortDescription }, func(s string) { result.ShortDescription = s }},
		{"description", func() string { return result.Description }, func(s string) { result.Description = s }},
		{"technicalData", func() string { return result.TechnicalData }, func(s string) { result.TechnicalData = s }},
		{"metaDescription", func() string { return result.MetaDescription }, func(s string) { result.MetaDescription = s }},
		{"urlContent", func() string { return result.URLContent }, func(s string) { result.URLContent = s }},
		{"previewText", func() string { return result.PreviewText }, func(s string) { result.PreviewText = s }},
	}

	for _, acc := range accessors {
		constraint, ok := v.fields.Get(acc.name)
		if !ok {
			continue
		}

		value := acc.get()
		original := value

		// 1. HTML sanitization
		if constraint.PlainText {
			value = v.sanitizer.StripAll(value)
			if value != original {
				errs = append(errs, ValidationError{
					Field:   acc.name,
					Code:    "html_stripped",
					Message: fmt.Sprintf("HTML tags were stripped from plain-text field %s", acc.name),
					Level:   Warning,
				})
			}
		} else {
			value = v.sanitizer.SanitizeHTML(value)
		}

		// 2. Trim whitespace
		value = NormalizeWhitespace(value)

		// 3. URL slug sanitization for urlContent
		if acc.name == "urlContent" {
			value = SanitizeURLSlug(value)
		}

		// 4. Required check
		if constraint.Required && value == "" {
			errs = append(errs, ValidationError{
				Field:   acc.name,
				Code:    "required",
				Message: fmt.Sprintf("required field %s is empty", acc.name),
				Level:   Error,
			})
			acc.set(value)
			continue
		}

		// Skip further checks on empty non-required fields.
		if value == "" {
			acc.set(value)
			continue
		}

		// 5. MinLength check (use rune count, not byte length)
		runeCount := utf8.RuneCountInString(value)
		if constraint.MinLength > 0 && runeCount < constraint.MinLength {
			errs = append(errs, ValidationError{
				Field:   acc.name,
				Code:    "too_short",
				Message: fmt.Sprintf("field %s has %d characters, minimum is %d", acc.name, runeCount, constraint.MinLength),
				Level:   Error,
			})
		}

		// 6. MaxLength check (truncate if over, add warning)
		if constraint.MaxLength > 0 && runeCount > constraint.MaxLength {
			runes := []rune(value)
			value = string(runes[:constraint.MaxLength])
			errs = append(errs, ValidationError{
				Field:   acc.name,
				Code:    "truncated",
				Message: fmt.Sprintf("field %s truncated from %d to %d characters", acc.name, runeCount, constraint.MaxLength),
				Level:   Warning,
			})
		}

		// 7. Language check
		if constraint.CheckLanguage && lang != "" {
			detected, matches := v.langDetector.Check(value, lang)
			if !matches {
				errs = append(errs, ValidationError{
					Field:   acc.name,
					Code:    "wrong_language",
					Message: fmt.Sprintf("field %s expected language %s but detected %s", acc.name, lang, detected),
					Level:   Warning,
				})
			}
		}

		acc.set(value)
	}

	return &result, errs
}

// ValidatePropertyValues validates each property value against its spec.
// Returns a copy and any validation errors.
func (v *Validator) ValidatePropertyValues(values *generate.PropertyValues, specs []generate.PropertySpec) (*generate.PropertyValues, []ValidationError) {
	// Create a copy to avoid mutating the original.
	result := generate.PropertyValues{
		Values: make([]generate.PropertyValue, len(values.Values)),
	}
	copy(result.Values, values.Values)

	var errs []ValidationError

	// Build a spec map for lookup by property ID.
	specMap := make(map[int64]generate.PropertySpec, len(specs))
	for _, s := range specs {
		specMap[s.ID] = s
	}

	for i, pv := range result.Values {
		spec, ok := specMap[pv.PropertyID]
		if !ok {
			continue
		}

		switch spec.PropertyType {
		case "int":
			if pv.IntValue == nil {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("property_%d", pv.PropertyID),
					Code:    "missing_value",
					Message: fmt.Sprintf("int property %q (ID %d) has no IntValue", spec.Name, spec.ID),
					Level:   Error,
				})
			}
		case "float":
			if pv.FloatValue == nil {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("property_%d", pv.PropertyID),
					Code:    "missing_value",
					Message: fmt.Sprintf("float property %q (ID %d) has no FloatValue", spec.Name, spec.ID),
					Level:   Error,
				})
			}
		case "selection":
			if len(spec.Options) > 0 {
				found := false
				for _, opt := range spec.Options {
					if opt == pv.SelectionValue {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("property_%d", pv.PropertyID),
						Code:    "invalid_selection",
						Message: fmt.Sprintf("selection property %q (ID %d) value %q is not in allowed options %v", spec.Name, spec.ID, pv.SelectionValue, spec.Options),
						Level:   Error,
					})
				}
			}
		case "text":
			if pv.TextValue == "" {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("property_%d", pv.PropertyID),
					Code:    "missing_value",
					Message: fmt.Sprintf("text property %q (ID %d) has empty TextValue", spec.Name, spec.ID),
					Level:   Error,
				})
			}
		}

		_ = i // result values are already copied
	}

	return &result, errs
}
