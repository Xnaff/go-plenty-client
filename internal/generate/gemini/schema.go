package gemini

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
	"github.com/janemig/plentyone/internal/generate"
)

// generateSchema derives a JSON schema map from a Go struct type using
// the invopop/jsonschema reflector. The result is cleaned for Gemini's
// ResponseJsonSchema, which uses an OpenAPI 3.0 subset and does not
// support standard JSON Schema meta-fields like $schema and $id.
func generateSchema[T any]() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)

	data, _ := json.Marshal(schema)
	var result map[string]any
	_ = json.Unmarshal(data, &result)

	// Strip JSON Schema meta-fields unsupported by Gemini's OpenAPI subset.
	cleanSchema(result)

	return result
}

// cleanSchema recursively removes JSON Schema meta-fields that are
// unsupported by Gemini's structured output (OpenAPI 3.0 subset).
func cleanSchema(m map[string]any) {
	delete(m, "$schema")
	delete(m, "$id")
	delete(m, "$ref")
	delete(m, "$defs")

	// Recurse into nested objects.
	for _, v := range m {
		switch val := v.(type) {
		case map[string]any:
			cleanSchema(val)
		case []any:
			for _, item := range val {
				if obj, ok := item.(map[string]any); ok {
					cleanSchema(obj)
				}
			}
		}
	}
}

// productTextsSchema is the JSON schema for structured output of product texts.
var productTextsSchema = generateSchema[generate.ProductTexts]()

// propertyValuesSchema is the JSON schema for structured output of property values.
var propertyValuesSchema = generateSchema[generate.PropertyValues]()

// priceResultSchema is the JSON schema for structured output of price generation.
var priceResultSchema = generateSchema[generate.PriceResult]()

// batchProductResultSchema is the JSON schema for batch product generation.
var batchProductResultSchema = generateSchema[generate.BatchProductResult]()
