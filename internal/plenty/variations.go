package plenty

import (
	"context"
	"fmt"
	"net/http"
)

// VariationService provides methods for the PlentyONE variations API.
type VariationService struct {
	client *Client
}

// Create creates a new variation for the given item.
// POST /rest/items/{itemId}/variations
func (s *VariationService) Create(ctx context.Context, itemID int64, req *CreateVariationRequest) (*Variation, error) {
	path := fmt.Sprintf("/rest/items/%d/variations", itemID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &Variation{ID: -1}, nil
	}

	return doJSON[Variation](ctx, s.client, http.MethodPost, path, req)
}

// Update updates an existing variation.
// PUT /rest/items/{itemId}/variations/{variationId}
func (s *VariationService) Update(ctx context.Context, itemID, variationID int64, req *UpdateVariationRequest) (*Variation, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d", itemID, variationID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPut, path, req)
		return &Variation{ID: variationID, ItemID: itemID}, nil
	}

	return doJSON[Variation](ctx, s.client, http.MethodPut, path, req)
}

// List retrieves a paginated list of variations for the given item.
// GET /rest/items/{itemId}/variations?page=X&itemsPerPage=Y
func (s *VariationService) List(ctx context.Context, itemID int64, params PaginationParams) (*PaginatedResponse[Variation], error) {
	path := fmt.Sprintf("/rest/items/%d/variations?page=%d&itemsPerPage=%d", itemID, params.Page, params.ItemsPerPage)
	return doJSON[PaginatedResponse[Variation]](ctx, s.client, http.MethodGet, path, nil)
}

// ListSalesPriceConfigs retrieves all sales price configurations.
// GET /rest/items/sales_prices
func (s *VariationService) ListSalesPriceConfigs(ctx context.Context) ([]SalesPriceConfig, error) {
	path := "/rest/items/sales_prices"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodGet, path, nil)
		return []SalesPriceConfig{
			{ID: 1, Type: "default"},
			{ID: 2, Type: "rrp", Position: 1},
			{ID: 3, Type: "default", Position: 2, Names: []SalesPriceName{
				{Lang: "en", NameInternal: "B2B Price", NameExternal: "B2B Price"},
				{Lang: "de", NameInternal: "B2B Preis", NameExternal: "B2B Preis"},
			}},
			{ID: 4, Type: "rrp", Position: 3, Names: []SalesPriceName{
				{Lang: "en", NameInternal: "B2B RRP", NameExternal: "B2B RRP"},
				{Lang: "de", NameInternal: "B2B UVP", NameExternal: "B2B UVP"},
			}},
		}, nil
	}

	var configs []SalesPriceConfig
	if err := s.client.do(ctx, http.MethodGet, path, nil, &configs); err != nil {
		return nil, fmt.Errorf("listing sales price configs: %w", err)
	}
	return configs, nil
}

// SetSalesPrice sets a price on a variation for a given sales price config.
// POST /rest/items/{itemId}/variations/{variationId}/variation_sales_prices
func (s *VariationService) SetSalesPrice(ctx context.Context, itemID, variationID int64, req *VariationSalesPriceRequest) (*VariationSalesPrice, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d/variation_sales_prices", itemID, variationID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &VariationSalesPrice{SalesPriceID: req.SalesPriceID, Price: req.Price, VariationID: variationID}, nil
	}

	return doJSON[VariationSalesPrice](ctx, s.client, http.MethodPost, path, req)
}

// Delete removes a variation from the given item.
// DELETE /rest/items/{itemId}/variations/{variationId}
func (s *VariationService) Delete(ctx context.Context, itemID, variationID int64) error {
	path := fmt.Sprintf("/rest/items/%d/variations/%d", itemID, variationID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodDelete, path, nil)
		return nil
	}

	resp, err := s.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseErrorResponse(resp)
	}

	return nil
}

// ListUnits retrieves all available units of measurement.
// GET /rest/items/units
func (s *VariationService) ListUnits(ctx context.Context) ([]Unit, error) {
	path := "/rest/items/units"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodGet, path, nil)
		return []Unit{
			{ID: 1, UnitOfMeasurement: "C62"},
			{ID: 2, UnitOfMeasurement: "KGM"},
			{ID: 3, UnitOfMeasurement: "GRM"},
			{ID: 4, UnitOfMeasurement: "LTR"},
			{ID: 5, UnitOfMeasurement: "MLT"},
			{ID: 6, UnitOfMeasurement: "MTR"},
		}, nil
	}

	var units []Unit
	if err := s.client.do(ctx, http.MethodGet, path, nil, &units); err != nil {
		return nil, fmt.Errorf("listing units: %w", err)
	}
	return units, nil
}

// ListBarcodeConfigs retrieves all barcode configurations.
// GET /rest/items/barcodes
func (s *VariationService) ListBarcodeConfigs(ctx context.Context) ([]BarcodeConfig, error) {
	path := "/rest/items/barcodes"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodGet, path, nil)
		return []BarcodeConfig{{ID: 1, Name: "EAN", Type: "GTIN_13"}}, nil
	}

	var configs []BarcodeConfig
	if err := s.client.do(ctx, http.MethodGet, path, nil, &configs); err != nil {
		return nil, fmt.Errorf("listing barcode configs: %w", err)
	}
	return configs, nil
}

// SetBarcode assigns a barcode to a variation.
// POST /rest/items/{itemId}/variations/{variationId}/variation_barcodes
func (s *VariationService) SetBarcode(ctx context.Context, itemID, variationID int64, req *CreateVariationBarcodeRequest) (*VariationBarcode, error) {
	path := fmt.Sprintf("/rest/items/%d/variations/%d/variation_barcodes", itemID, variationID)

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &VariationBarcode{BarcodeID: req.BarcodeID, Code: req.Code, VariationID: variationID}, nil
	}

	return doJSON[VariationBarcode](ctx, s.client, http.MethodPost, path, req)
}

// CreateSalesPriceConfig creates a new sales price configuration.
// POST /rest/items/sales_prices
func (s *VariationService) CreateSalesPriceConfig(ctx context.Context, req *CreateSalesPriceConfigRequest) (*SalesPriceConfig, error) {
	path := "/rest/items/sales_prices"

	if s.client.dryRun {
		dryRunLog(s.client.logger, http.MethodPost, path, req)
		return &SalesPriceConfig{ID: -1, Type: req.Type, Position: req.Position, MinimumOrderQuantity: req.MinimumOrderQuantity}, nil
	}

	return doJSON[SalesPriceConfig](ctx, s.client, http.MethodPost, path, req)
}
