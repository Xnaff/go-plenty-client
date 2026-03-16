package plenty

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

// PaginationParams configures paginated list requests.
type PaginationParams struct {
	Page         int `json:"page"`
	ItemsPerPage int `json:"itemsPerPage"`
}

// PaginatedResponse is a generic wrapper for paginated API responses.
type PaginatedResponse[T any] struct {
	Page        int  `json:"page"`
	TotalsCount int  `json:"totalsCount"`
	IsLastPage  bool `json:"isLastPage"`
	Entries     []T  `json:"entries"`
}

// ---------------------------------------------------------------------------
// Category types
// ---------------------------------------------------------------------------

// CreateCategoryRequest is the payload for POST /rest/categories.
type CreateCategoryRequest struct {
	ParentCategoryID *int64           `json:"parentCategoryId,omitempty"`
	Type             string           `json:"type"` // default "item"
	Details          []CategoryDetail `json:"details"`
}

// CategoryDetail holds one language's name/description for a category.
type CategoryDetail struct {
	Lang        string `json:"lang"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Category is the response struct returned by the PlentyONE categories API.
type Category struct {
	ID               int64            `json:"id"`
	ParentCategoryID *int64           `json:"parentCategoryId,omitempty"`
	Type             string           `json:"type"`
	Details          []CategoryDetail `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Attribute types
// ---------------------------------------------------------------------------

// CreateAttributeRequest is the payload for POST /rest/attributes.
type CreateAttributeRequest struct {
	BackendName          string `json:"backendName"`
	Position             int    `json:"position"`
	IsSurchargePercental bool   `json:"isSurchargePercental"`
}

// Attribute is the response struct from the attributes API.
type Attribute struct {
	ID          int64  `json:"id"`
	BackendName string `json:"backendName"`
	Position    int    `json:"position"`
}

// CreateAttributeValueRequest is the payload for POST /rest/attributes/{id}/values.
type CreateAttributeValueRequest struct {
	BackendName string `json:"backendName"`
	Position    int    `json:"position"`
}

// AttributeValue is the response struct for an attribute value.
type AttributeValue struct {
	ID          int64  `json:"id"`
	AttributeID int64  `json:"attributeId"`
	BackendName string `json:"backendName"`
	Position    int    `json:"position"`
}

// CreateAttributeNameRequest is the payload for POST /rest/items/attributes/{id}/names.
type CreateAttributeNameRequest struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

// AttributeName is the response struct for an attribute name translation.
type AttributeName struct {
	ID          int64  `json:"id"`
	AttributeID int64  `json:"attributeId"`
	Lang        string `json:"lang"`
	Name        string `json:"name"`
}

// CreateAttributeValueNameRequest is the payload for POST /rest/items/attribute_values/{id}/names.
type CreateAttributeValueNameRequest struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

// AttributeValueName is the response struct for an attribute value name translation.
type AttributeValueName struct {
	ID      int64  `json:"id"`
	ValueID int64  `json:"valueId"`
	Lang    string `json:"lang"`
	Name    string `json:"name"`
}

// ---------------------------------------------------------------------------
// Property types
// ---------------------------------------------------------------------------

// CreatePropertyRequest is the payload for POST /rest/properties.
type CreatePropertyRequest struct {
	Cast     string         `json:"cast"` // text, int, float, selection
	Position int            `json:"position"`
	Names    []PropertyName `json:"names"`
}

// PropertyName holds one language's name for a property.
type PropertyName struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

// Property is the response struct from the properties API.
type Property struct {
	ID       int64          `json:"id"`
	Cast     string         `json:"cast"`
	Position int            `json:"position"`
	Names    []PropertyName `json:"names,omitempty"`
}

// CreatePropertySelectionRequest is the payload for POST /rest/properties/{id}/selections.
type CreatePropertySelectionRequest struct {
	Names []PropertyName `json:"names"`
}

// PropertySelection is the response struct for a property selection value.
type PropertySelection struct {
	ID         int64          `json:"id"`
	PropertyID int64          `json:"propertyId"`
	Names      []PropertyName `json:"names,omitempty"`
}

// PropertyRelationRequest is the payload for POST /rest/properties/relations.
type PropertyRelationRequest struct {
	RelationTargetID         int64  `json:"relationTargetId"`
	RelationTypeIdentifier   string `json:"relationTypeIdentifier"`
}

// PropertyRelation is the response struct for a property relation.
type PropertyRelation struct {
	ID                       int64  `json:"id"`
	RelationTargetID         int64  `json:"relationTargetId"`
	RelationTypeIdentifier   string `json:"relationTypeIdentifier"`
}

// ---------------------------------------------------------------------------
// Property Group types
// ---------------------------------------------------------------------------

// CreatePropertyGroupRequest is the payload for POST /rest/properties/groups.
type CreatePropertyGroupRequest struct {
	Position int `json:"position"`
}

// PropertyGroup is the response struct from the property groups API.
type PropertyGroup struct {
	ID       int64 `json:"id"`
	Position int   `json:"position"`
}

// CreatePropertyGroupNameRequest is the payload for POST /rest/properties/groups/names.
type CreatePropertyGroupNameRequest struct {
	PropertyGroupID int64  `json:"propertyGroupId"`
	Lang            string `json:"lang"`
	Name            string `json:"name"`
}

// PropertyGroupName is the response struct for a property group name.
type PropertyGroupName struct {
	ID              int64  `json:"id"`
	PropertyGroupID int64  `json:"propertyGroupId"`
	Lang            string `json:"lang"`
	Name            string `json:"name"`
}

// ---------------------------------------------------------------------------
// Item types
// ---------------------------------------------------------------------------

// CreateItemRequest is the payload for POST /rest/items.
// An item is created together with its main variation.
type CreateItemRequest struct {
	Variations []CreateItemVariation `json:"variations"`
}

// CreateItemVariation represents a variation within a CreateItemRequest.
type CreateItemVariation struct {
	VariationDefaultCategory CreateItemCategory `json:"variationDefaultCategory"`
	Name                     string             `json:"name"`
	Number                   string             `json:"number,omitempty"`
}

// CreateItemCategory identifies the default category for a variation.
type CreateItemCategory struct {
	CategoryID int64 `json:"categoryId"`
}

// Item is the response struct from the items API.
type Item struct {
	ID         int64       `json:"id"`
	Variations []Variation `json:"variations,omitempty"`
}

// ---------------------------------------------------------------------------
// Variation types
// ---------------------------------------------------------------------------

// CreateVariationRequest is the payload for POST /rest/items/{itemId}/variations.
type CreateVariationRequest struct {
	Name                     string                    `json:"name,omitempty"`
	Number                   string                    `json:"number,omitempty"`
	VariationAttributeValues []VariationAttributeValue `json:"variationAttributeValues,omitempty"`
}

// VariationAttributeValue links an attribute and value to a variation.
type VariationAttributeValue struct {
	AttributeID int64 `json:"attributeId"`
	ValueID     int64 `json:"valueId"`
}

// Variation is the response struct for a variation.
type Variation struct {
	ID                       int64                     `json:"id"`
	ItemID                   int64                     `json:"itemId"`
	Name                     string                    `json:"name,omitempty"`
	Number                   string                    `json:"number,omitempty"`
	VariationAttributeValues []VariationAttributeValue `json:"variationAttributeValues,omitempty"`
}

// UpdateVariationRequest is the payload for PUT /rest/items/{itemId}/variations/{variationId}.
type UpdateVariationRequest struct {
	Name       string `json:"name,omitempty"`
	Number     string `json:"number,omitempty"`
	IsActive   *bool  `json:"isActive,omitempty"`
	ExternalID string `json:"externalId,omitempty"`    // External variation ID
	Model      string `json:"model,omitempty"`         // Product model identifier
	Position   *int   `json:"position,omitempty"`      // Display ordering
	WeightG    *int   `json:"weightG,omitempty"`       // Gross weight in grams
	WeightNetG *int   `json:"weightNetG,omitempty"`    // Net weight in grams
	LengthMM   *int   `json:"lengthMM,omitempty"`      // Length in millimeters
	WidthMM    *int   `json:"widthMM,omitempty"`        // Width in millimeters
	HeightMM   *int   `json:"heightMM,omitempty"`       // Height in millimeters
	UnitID     *int64 `json:"unitId,omitempty"`          // PlentyONE unit ID
}

// ---------------------------------------------------------------------------
// Sales Price types
// ---------------------------------------------------------------------------

// SalesPriceConfig represents a sales price configuration in PlentyONE.
// Each shop needs at least one sales price config (e.g., "Default price").
// GET /rest/items/sales_prices
type SalesPriceConfig struct {
	ID                   int64            `json:"id"`
	Position             int              `json:"position"`
	Type                 string           `json:"type"` // "default", "rrp", "specialOffer"
	MinimumOrderQuantity float64          `json:"minimumOrderQuantity,omitempty"`
	Names                []SalesPriceName `json:"names,omitempty"`
}

// CreateSalesPriceConfigRequest is the payload for POST /rest/items/sales_prices.
type CreateSalesPriceConfigRequest struct {
	Type                 string           `json:"type"`                           // "default", "rrp", "specialOffer"
	Position             int              `json:"position,omitempty"`
	MinimumOrderQuantity float64          `json:"minimumOrderQuantity,omitempty"` // For graduated pricing tiers
	Names                []SalesPriceName `json:"names,omitempty"`
}

// SalesPriceName holds a multilingual name for a sales price config.
type SalesPriceName struct {
	Lang         string `json:"lang"`
	NameInternal string `json:"nameInternal,omitempty"`
	NameExternal string `json:"nameExternal,omitempty"`
}

// VariationSalesPriceRequest links a price value to a variation via a sales price config.
// POST /rest/items/{itemId}/variations/{variationId}/variation_sales_prices
type VariationSalesPriceRequest struct {
	SalesPriceID int64   `json:"salesPriceId"`
	Price        float64 `json:"price"`
}

// VariationSalesPrice is the response when getting/setting a sales price on a variation.
type VariationSalesPrice struct {
	SalesPriceID int64   `json:"salesPriceId"`
	Price        float64 `json:"price"`
	VariationID  int64   `json:"variationId"`
}

// ---------------------------------------------------------------------------
// Image types
// ---------------------------------------------------------------------------

// UploadImageBase64Request is the payload for uploading an image via base64 JSON.
// POST /rest/items/{id}/images/upload
type UploadImageBase64Request struct {
	UploadImageData string `json:"uploadImageData"` // base64-encoded image data
	Position        int    `json:"position"`
}

// UploadImageURLRequest is the payload for uploading an image via URL.
// POST /rest/items/{id}/images/upload
type UploadImageURLRequest struct {
	UploadURL string `json:"uploadUrl"`
	Position  int    `json:"position"`
}

// Image is the response struct for an item image.
type Image struct {
	ID       int64  `json:"id"`
	ItemID   int64  `json:"itemId"`
	Position int    `json:"position"`
	URL      string `json:"url,omitempty"`
}

// ---------------------------------------------------------------------------
// Text / Description types
// ---------------------------------------------------------------------------

// CreateDescriptionRequest is the payload for creating or updating a variation
// description. ONE language per call (not all-at-once).
// POST /rest/items/{id}/variations/{variationId}/descriptions
type CreateDescriptionRequest struct {
	Lang             string `json:"lang"`
	Name             string `json:"name"`
	ShortDescription string `json:"shortDescription,omitempty"`
	Description      string `json:"description,omitempty"`
	TechnicalData    string `json:"technicalData,omitempty"`
	MetaDescription  string `json:"metaDescription,omitempty"`
	URLContent       string `json:"urlContent,omitempty"`
}

// Description is the response struct for a variation description.
type Description struct {
	ItemID           int64  `json:"itemId"`
	VariationID      int64  `json:"variationId"`
	Lang             string `json:"lang"`
	Name             string `json:"name"`
	ShortDescription string `json:"shortDescription,omitempty"`
	Description      string `json:"description,omitempty"`
	TechnicalData    string `json:"technicalData,omitempty"`
	MetaDescription  string `json:"metaDescription,omitempty"`
	URLContent       string `json:"urlContent,omitempty"`
}

// ---------------------------------------------------------------------------
// Unit types
// ---------------------------------------------------------------------------

// Unit represents a unit of measurement in PlentyONE.
// GET /rest/items/units
type Unit struct {
	ID                int64      `json:"id"`
	UnitOfMeasurement string     `json:"unitOfMeasurement"` // ISO code: "C62" (piece), "KGM" (kg), "GRM" (g), "LTR" (liter), "MTR" (meter), "MLT" (ml)
	IsDecimalPlaces   bool       `json:"isDecimalPlaces"`
	Names             []UnitName `json:"names,omitempty"`
}

// UnitName holds one language's display name for a unit.
type UnitName struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

// ---------------------------------------------------------------------------
// Barcode types
// ---------------------------------------------------------------------------

// BarcodeConfig represents a barcode configuration in PlentyONE.
// GET /rest/items/barcodes
type BarcodeConfig struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "GTIN_13", "GTIN_128", "UPC", "ISBN"
}

// CreateVariationBarcodeRequest links a barcode code to a variation.
// POST /rest/items/{itemId}/variations/{variationId}/variation_barcodes
type CreateVariationBarcodeRequest struct {
	BarcodeID int64  `json:"barcodeId"`
	Code      string `json:"code"`
}

// VariationBarcode is the response for a variation barcode.
type VariationBarcode struct {
	BarcodeID   int64  `json:"barcodeId"`
	Code        string `json:"code"`
	VariationID int64  `json:"variationId"`
}
