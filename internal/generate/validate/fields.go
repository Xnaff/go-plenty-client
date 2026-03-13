package validate

// FieldConstraint defines validation rules for a single product text field.
type FieldConstraint struct {
	MaxLength       int      // 0 = unlimited
	MinLength       int
	PlainText       bool     // If true, strip all HTML tags
	CheckLanguage   bool     // If true, validate detected language matches expected
	Required        bool     // If true, empty string is an error
	AllowedHTMLTags []string // For non-plain-text fields; nil = no restriction
}

// FieldConstraints holds all field constraint definitions.
type FieldConstraints struct {
	constraints map[string]FieldConstraint
}

// Get returns the constraint for a field name and whether it exists.
func (fc FieldConstraints) Get(field string) (FieldConstraint, bool) {
	c, ok := fc.constraints[field]
	return c, ok
}

// DefaultFieldConstraints returns the field constraints matching PlentyONE API requirements.
func DefaultFieldConstraints() FieldConstraints {
	return FieldConstraints{
		constraints: map[string]FieldConstraint{
			"name": {
				MaxLength:     240,
				MinLength:     3,
				PlainText:     true,
				CheckLanguage: true,
				Required:      true,
			},
			"shortDescription": {
				MaxLength:     500,
				MinLength:     10,
				PlainText:     true,
				CheckLanguage: true,
				Required:      false,
			},
			"description": {
				MaxLength:     0, // unlimited
				MinLength:     50,
				PlainText:     false,
				CheckLanguage: true,
				Required:      true,
			},
			"technicalData": {
				MaxLength:     0, // unlimited
				MinLength:     0,
				PlainText:     false,
				CheckLanguage: false,
				Required:      false,
			},
			"metaDescription": {
				MaxLength:     255,
				MinLength:     50,
				PlainText:     true,
				CheckLanguage: true,
				Required:      true,
			},
			"urlContent": {
				MaxLength:     240,
				MinLength:     3,
				PlainText:     true,
				CheckLanguage: false,
				Required:      true,
			},
			"previewText": {
				MaxLength:     200,
				MinLength:     5,
				PlainText:     true,
				CheckLanguage: true,
				Required:      false,
			},
		},
	}
}
