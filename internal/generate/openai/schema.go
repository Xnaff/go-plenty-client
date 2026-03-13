package openai

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
	"github.com/janemig/plentyone/internal/generate"
)

// generateSchema derives a JSON schema map from a Go struct type using
// the invopop/jsonschema reflector. This is the recommended approach from
// the openai-go SDK documentation for structured output.
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
	return result
}

// productTextsSchema is the JSON schema for structured output of product texts.
// Generated once at package initialization from the Go struct.
var productTextsSchema = generateSchema[generate.ProductTexts]()

// propertyValuesSchema is the JSON schema for structured output of property values.
// Generated once at package initialization from the Go struct.
var propertyValuesSchema = generateSchema[generate.PropertyValues]()

// priceResultSchema is the JSON schema for structured output of price generation.
// Generated once at package initialization from the Go struct.
var priceResultSchema = generateSchema[generate.PriceResult]()
