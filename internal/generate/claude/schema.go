package claude

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
	"github.com/janemig/plentyone/internal/generate"
)

// generateSchema derives an anthropic.ToolInputSchemaParam from a Go struct
// type using the invopop/jsonschema reflector. This follows the pattern from
// the anthropic-sdk-go README for tool-based structured output.
func generateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

// productTextsSchema is the tool input schema for structured output of product texts.
var productTextsSchema = generateSchema[generate.ProductTexts]()

// propertyValuesSchema is the tool input schema for structured output of property values.
var propertyValuesSchema = generateSchema[generate.PropertyValues]()

// priceResultSchema is the tool input schema for structured output of price generation.
var priceResultSchema = generateSchema[generate.PriceResult]()

// batchProductResultSchema is the tool input schema for batch product generation.
var batchProductResultSchema = generateSchema[generate.BatchProductResult]()
