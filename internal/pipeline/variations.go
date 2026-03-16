package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage/queries"
)

// VariationStage creates additional variations (beyond the main variation) in
// PlentyONE for each product, then sets sales prices (default + RRP + B2B),
// barcodes, weight/units/dimensions, variation metadata, and graduated prices
// on ALL variations.
//
// The main variation is created together with the item in the products stage,
// so this stage handles only the 2nd+ variations for creation. All other
// operations apply to all variations.
//
// Phases:
//  1. Create additional variations (index 1+)
//  2. Set selling price + RRP + B2B prices + update physical data + variation metadata
//  3. Set barcodes
//  4. Set graduated prices (selling + RRP per tier)
type VariationStage struct {
	client              *plenty.Client
	db                  *queries.Queries
	cfg                 PipelineConfig
	defaultSalesPriceID int64 // cached after first lookup
	rrpSalesPriceID     int64 // cached after first lookup
	b2bDefaultPriceID   int64 // cached after first lookup
	b2bRrpPriceID       int64 // cached after first lookup
}

// NewVariationStage creates a VariationStage ready for registration.
func NewVariationStage(client *plenty.Client, db *queries.Queries, cfg PipelineConfig) *VariationStage {
	return &VariationStage{client: client, db: db, cfg: cfg}
}

// Name returns the stage identifier.
func (s *VariationStage) Name() domain.StageName {
	return domain.StageVariations
}

// variationItem pairs a local variation with its parent product for concurrent processing.
type variationItem struct {
	variation queries.Variation
	product   queries.Product
}

// priceItem pairs a local variation with the PlentyONE IDs needed for price setting.
type priceItem struct {
	variation    queries.Variation
	plentyItemID int64
	plentyVarID  int64
	salesPriceID int64
	price        float64
}

// barcodeItem pairs a local variation with the PlentyONE IDs for barcode setting.
type barcodeItem struct {
	variation     queries.Variation
	plentyItemID  int64
	plentyVarID   int64
	barcodeConfID int64
	code          string
}

// graduatedPriceItem pairs a local variation with one graduated price tier.
type graduatedPriceItem struct {
	variation    queries.Variation
	plentyItemID int64
	plentyVarID  int64
	salesPriceID int64
	price        float64
	minQty       int
}

// resolvedVariation holds the PlentyONE IDs for a successfully mapped variation.
type resolvedVariation struct {
	item         variationItem
	plentyItemID int64
	plentyVarID  int64
}

// Execute creates additional variations for every product in the job, then
// sets prices, barcodes, weight/units, and graduated prices.
func (s *VariationStage) Execute(ctx context.Context, rc *RunContext) error {
	products, err := s.db.ListProductsByJob(ctx, rc.JobID)
	if err != nil {
		return fmt.Errorf("listing products for job %d: %w", rc.JobID, err)
	}

	// Phase 1: Create additional variations (index 1+).
	var createItems []variationItem
	// Also collect ALL variations for subsequent phases.
	var allVariations []variationItem

	for _, product := range products {
		variations, err := s.db.ListVariationsByProduct(ctx, product.ID)
		if err != nil {
			rc.Logger.Error("failed to list variations for product",
				slog.Int64("product_id", product.ID),
				slog.Any("error", err),
			)
			continue
		}

		for i, v := range variations {
			item := variationItem{variation: v, product: product}
			allVariations = append(allVariations, item)
			if i > 0 {
				// Additional variations need creation.
				createItems = append(createItems, item)
			}
		}
	}

	// Phase 1: Create additional variations.
	if len(createItems) > 0 {
		succeeded, failed := ProcessItems(ctx, createItems, s.cfg.Concurrency, func(ctx context.Context, item variationItem) error {
			return s.processVariation(ctx, rc, item)
		})

		rc.Logger.Info("variation creation complete",
			slog.Int("succeeded", succeeded),
			slog.Int("failed", failed),
		)
	} else {
		rc.Logger.Info("no additional variations to create")
	}

	// Resolve PlentyONE IDs for all variations (needed by phases 2-4).
	resolved := s.resolveVariations(ctx, rc, allVariations)
	if len(resolved) == 0 {
		rc.Logger.Info("no resolved variations, skipping prices/barcodes/graduated prices")
		return nil
	}

	// Phase 2: Set selling price + RRP + update weight/unit.
	if err := s.executePricePhase(ctx, rc, resolved); err != nil {
		rc.Logger.Warn("price phase encountered issues", "error", err)
	}

	// Phase 3: Set barcodes.
	if err := s.executeBarcodePhase(ctx, rc, resolved); err != nil {
		rc.Logger.Warn("barcode phase encountered issues", "error", err)
	}

	// Phase 4: Set graduated prices.
	if err := s.executeGraduatedPricePhase(ctx, rc, resolved); err != nil {
		rc.Logger.Warn("graduated price phase encountered issues", "error", err)
	}

	return nil
}

// resolveVariations resolves PlentyONE item/variation IDs for all local variations.
func (s *VariationStage) resolveVariations(ctx context.Context, rc *RunContext, items []variationItem) []resolvedVariation {
	var resolved []resolvedVariation
	for _, item := range items {
		varMapping, mErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
			RunID:      rc.RunID,
			LocalID:    item.variation.ID,
			EntityType: string(domain.EntityVariation),
		})
		if mErr != nil || varMapping.Status != string(domain.StatusCreated) {
			continue
		}

		prodMapping, pErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
			RunID:      rc.RunID,
			LocalID:    item.product.ID,
			EntityType: string(domain.EntityProduct),
		})
		if pErr != nil {
			continue
		}

		resolved = append(resolved, resolvedVariation{
			item:         item,
			plentyItemID: prodMapping.PlentyID,
			plentyVarID:  varMapping.PlentyID,
		})
	}
	return resolved
}

// ---------------------------------------------------------------------------
// Phase 2: Prices (default + RRP) + weight/unit update
// ---------------------------------------------------------------------------

func (s *VariationStage) executePricePhase(ctx context.Context, rc *RunContext, resolved []resolvedVariation) error {
	// Resolve sales price configs.
	configs, err := s.client.Variations.ListSalesPriceConfigs(ctx)
	if err != nil {
		return fmt.Errorf("listing sales price configs: %w", err)
	}

	// Build config maps by type. For retail, pick the first non-B2B config of each type.
	var defaultPriceID, rrpPriceID int64
	var b2bDefaultID, b2bRrpID int64
	for _, c := range configs {
		isB2B := isB2BConfig(c)
		if c.MinimumOrderQuantity > 0 {
			continue // skip graduated configs
		}
		switch c.Type {
		case "default":
			if isB2B {
				b2bDefaultID = c.ID
			} else if defaultPriceID == 0 {
				defaultPriceID = c.ID
			}
		case "rrp":
			if isB2B {
				b2bRrpID = c.ID
			} else if rrpPriceID == 0 {
				rrpPriceID = c.ID
			}
		}
	}

	if defaultPriceID == 0 && len(configs) > 0 {
		defaultPriceID = configs[0].ID
	}
	if defaultPriceID == 0 {
		rc.Logger.Warn("no sales price configuration found, skipping price setting")
		return nil
	}

	// If no RRP config exists, create one.
	if rrpPriceID == 0 {
		created, cErr := s.client.Variations.CreateSalesPriceConfig(ctx, &plenty.CreateSalesPriceConfigRequest{
			Type:     "rrp",
			Position: 1,
		})
		if cErr != nil {
			rc.Logger.Warn("could not create RRP sales price config", "error", cErr)
		} else {
			rrpPriceID = created.ID
			rc.Logger.Info("created RRP sales price config", "id", rrpPriceID)
		}
	}

	// Create B2B default config if missing.
	if b2bDefaultID == 0 {
		created, cErr := s.client.Variations.CreateSalesPriceConfig(ctx, &plenty.CreateSalesPriceConfigRequest{
			Type:     "default",
			Position: 2,
			Names: []plenty.SalesPriceName{
				{Lang: "en", NameInternal: "B2B Price", NameExternal: "B2B Price"},
				{Lang: "de", NameInternal: "B2B Preis", NameExternal: "B2B Preis"},
			},
		})
		if cErr != nil {
			rc.Logger.Warn("could not create B2B default sales price config", "error", cErr)
		} else {
			b2bDefaultID = created.ID
			rc.Logger.Info("created B2B default sales price config", "id", b2bDefaultID)
		}
	}

	// Create B2B RRP config if missing.
	if b2bRrpID == 0 {
		created, cErr := s.client.Variations.CreateSalesPriceConfig(ctx, &plenty.CreateSalesPriceConfigRequest{
			Type:     "rrp",
			Position: 3,
			Names: []plenty.SalesPriceName{
				{Lang: "en", NameInternal: "B2B RRP", NameExternal: "B2B RRP"},
				{Lang: "de", NameInternal: "B2B UVP", NameExternal: "B2B UVP"},
			},
		})
		if cErr != nil {
			rc.Logger.Warn("could not create B2B RRP sales price config", "error", cErr)
		} else {
			b2bRrpID = created.ID
			rc.Logger.Info("created B2B RRP sales price config", "id", b2bRrpID)
		}
	}

	s.defaultSalesPriceID = defaultPriceID
	s.rrpSalesPriceID = rrpPriceID
	s.b2bDefaultPriceID = b2bDefaultID
	s.b2bRrpPriceID = b2bRrpID

	// Resolve unit mapping for weight/unit updates.
	unitMap := s.buildUnitMap(ctx, rc)

	// Build price items.
	var priceItems []priceItem
	for i, rv := range resolved {
		v := rv.item.variation

		// Set default selling price.
		if v.Price.Valid {
			price, parseErr := strconv.ParseFloat(v.Price.String, 64)
			if parseErr == nil && price > 0 {
				priceItems = append(priceItems, priceItem{
					variation:    v,
					plentyItemID: rv.plentyItemID,
					plentyVarID:  rv.plentyVarID,
					salesPriceID: defaultPriceID,
					price:        price,
				})
			}
		}

		// Set RRP price.
		if rrpPriceID > 0 && v.Rrp.Valid {
			rrp, parseErr := strconv.ParseFloat(v.Rrp.String, 64)
			if parseErr == nil && rrp > 0 {
				priceItems = append(priceItems, priceItem{
					variation:    v,
					plentyItemID: rv.plentyItemID,
					plentyVarID:  rv.plentyVarID,
					salesPriceID: rrpPriceID,
					price:        rrp,
				})
			}
		}

		// Set B2B selling price.
		if b2bDefaultID > 0 && v.B2bPrice.Valid {
			b2b, parseErr := strconv.ParseFloat(v.B2bPrice.String, 64)
			if parseErr == nil && b2b > 0 {
				priceItems = append(priceItems, priceItem{
					variation:    v,
					plentyItemID: rv.plentyItemID,
					plentyVarID:  rv.plentyVarID,
					salesPriceID: b2bDefaultID,
					price:        b2b,
				})
			}
		}

		// Set B2B RRP.
		if b2bRrpID > 0 && v.B2bRrp.Valid {
			b2bRrp, parseErr := strconv.ParseFloat(v.B2bRrp.String, 64)
			if parseErr == nil && b2bRrp > 0 {
				priceItems = append(priceItems, priceItem{
					variation:    v,
					plentyItemID: rv.plentyItemID,
					plentyVarID:  rv.plentyVarID,
					salesPriceID: b2bRrpID,
					price:        b2bRrp,
				})
			}
		}

		// Update variation with physical data and metadata.
		s.updateVariationData(ctx, rc, rv, unitMap, i)
	}

	if len(priceItems) == 0 {
		rc.Logger.Info("no variations with prices to set")
		return nil
	}

	priceSucceeded, priceFailed := ProcessItems(ctx, priceItems, s.cfg.Concurrency, func(ctx context.Context, item priceItem) error {
		_, err := s.client.Variations.SetSalesPrice(ctx, item.plentyItemID, item.plentyVarID, &plenty.VariationSalesPriceRequest{
			SalesPriceID: item.salesPriceID,
			Price:        item.price,
		})
		if err != nil {
			rc.Logger.Error("failed to set sales price",
				slog.Int64("variation_id", item.variation.ID),
				slog.Float64("price", item.price),
				slog.Any("error", err),
			)
			return err
		}
		rc.Logger.Debug("set sales price on variation",
			slog.Int64("variation_id", item.variation.ID),
			slog.Int64("plenty_variation_id", item.plentyVarID),
			slog.Float64("price", item.price),
			slog.Int64("sales_price_id", item.salesPriceID),
		)
		return nil
	})

	rc.Logger.Info("variation price setting complete",
		slog.Int("succeeded", priceSucceeded),
		slog.Int("failed", priceFailed),
	)

	return nil
}

// isB2BConfig checks if a sales price config is a B2B config by inspecting its names.
func isB2BConfig(c plenty.SalesPriceConfig) bool {
	for _, name := range c.Names {
		upper := strings.ToUpper(name.NameInternal + " " + name.NameExternal)
		if strings.Contains(upper, "B2B") {
			return true
		}
	}
	return false
}

// updateVariationData updates a variation's physical data (weight, unit, dimensions)
// and metadata (name, externalId, model, position) in PlentyONE.
func (s *VariationStage) updateVariationData(ctx context.Context, rc *RunContext, rv resolvedVariation, unitMap map[string]int64, variationIndex int) {
	v := rv.item.variation

	var weightG *int
	if v.Weight.Valid {
		w, err := strconv.ParseFloat(v.Weight.String, 64)
		if err == nil && w > 0 {
			// Convert to grams based on the unit stored.
			grams := weightToGrams(w, v.WeightUnit)
			g := int(math.Round(grams))
			weightG = &g
		}
	}

	var unitID *int64
	if v.SalesUnit != "" {
		if id, ok := unitMap[strings.ToLower(v.SalesUnit)]; ok {
			unitID = &id
		}
	}

	// Dimensions are stored in mm in the DB and sent as mm to PlentyONE.
	var lengthMM, widthMM, heightMM *int
	if v.LengthMm > 0 {
		l := int(v.LengthMm)
		lengthMM = &l
	}
	if v.WidthMm > 0 {
		w := int(v.WidthMm)
		widthMM = &w
	}
	if v.HeightMm > 0 {
		h := int(v.HeightMm)
		heightMM = &h
	}

	req := &plenty.UpdateVariationRequest{
		// 5 variation metadata fields.
		Name:       rv.item.product.Name,                       // Variantenname: product name
		ExternalID: fmt.Sprintf("%d", rv.item.variation.ID),    // Ext. Varianten-ID: local DB ID
		Model:      v.Model,                                     // Modell: AI-generated model
		Position:   intPtr(variationIndex),                      // Position: sequential index
		// Physical data.
		WeightG:    weightG,
		WeightNetG: weightG, // Use same value for net weight.
		LengthMM:   lengthMM,
		WidthMM:    widthMM,
		HeightMM:   heightMM,
		UnitID:     unitID,
	}

	if _, err := s.client.Variations.Update(ctx, rv.plentyItemID, rv.plentyVarID, req); err != nil {
		rc.Logger.Warn("failed to update variation data",
			slog.Int64("variation_id", v.ID),
			slog.Any("error", err),
		)
	} else {
		rc.Logger.Debug("updated variation data",
			slog.Int64("variation_id", v.ID),
			slog.Int64("plenty_variation_id", rv.plentyVarID),
		)
	}
}

// intPtr returns a pointer to the given int.
func intPtr(v int) *int {
	return &v
}

// weightToGrams converts a weight value to grams based on the unit.
func weightToGrams(weight float64, unit string) float64 {
	switch strings.ToLower(unit) {
	case "kg":
		return weight * 1000
	case "g":
		return weight
	default:
		return weight * 1000 // Default to kg.
	}
}

// buildUnitMap queries PlentyONE units and builds a mapping from common names
// to PlentyONE unit IDs. Maps both ISO codes and natural names.
func (s *VariationStage) buildUnitMap(ctx context.Context, rc *RunContext) map[string]int64 {
	units, err := s.client.Variations.ListUnits(ctx)
	if err != nil {
		rc.Logger.Warn("could not list units, skipping unit assignment", "error", err)
		return nil
	}

	// ISO UN/ECE unit code → common name mapping.
	isoToCommon := map[string][]string{
		"C62": {"piece", "pieces", "pcs"},
		"KGM": {"kg", "kilogram"},
		"GRM": {"g", "gram", "grams"},
		"LTR": {"liter", "litre", "l"},
		"MLT": {"ml", "milliliter"},
		"MTR": {"meter", "metre", "m"},
		"PR":  {"pair", "pairs"},
		"SET": {"set", "sets"},
		"BX":  {"box"},
		"PK":  {"pack", "package"},
	}

	unitMap := make(map[string]int64)
	for _, u := range units {
		// Map common names from the ISO code.
		if names, ok := isoToCommon[u.UnitOfMeasurement]; ok {
			for _, name := range names {
				unitMap[name] = u.ID
			}
		}
		// Also map the ISO code itself (lowercase).
		unitMap[strings.ToLower(u.UnitOfMeasurement)] = u.ID
	}

	return unitMap
}

// ---------------------------------------------------------------------------
// Phase 3: Barcodes
// ---------------------------------------------------------------------------

func (s *VariationStage) executeBarcodePhase(ctx context.Context, rc *RunContext, resolved []resolvedVariation) error {
	// Find the EAN/GTIN_13 barcode config.
	barcodeConfigs, err := s.client.Variations.ListBarcodeConfigs(ctx)
	if err != nil {
		return fmt.Errorf("listing barcode configs: %w", err)
	}

	var eanConfigID int64
	for _, bc := range barcodeConfigs {
		if bc.Type == "GTIN_13" || bc.Type == "EAN_13" {
			eanConfigID = bc.ID
			break
		}
	}
	if eanConfigID == 0 && len(barcodeConfigs) > 0 {
		// Fall back to first barcode config.
		eanConfigID = barcodeConfigs[0].ID
	}
	if eanConfigID == 0 {
		rc.Logger.Warn("no barcode configuration found in PlentyONE, skipping barcodes")
		return nil
	}

	// Build barcode items for variations with non-empty barcodes.
	var barcodeItems []barcodeItem
	for _, rv := range resolved {
		if rv.item.variation.Barcode == "" {
			continue
		}
		barcodeItems = append(barcodeItems, barcodeItem{
			variation:     rv.item.variation,
			plentyItemID:  rv.plentyItemID,
			plentyVarID:   rv.plentyVarID,
			barcodeConfID: eanConfigID,
			code:          rv.item.variation.Barcode,
		})
	}

	if len(barcodeItems) == 0 {
		rc.Logger.Info("no variations with barcodes to set")
		return nil
	}

	bcSucceeded, bcFailed := ProcessItems(ctx, barcodeItems, s.cfg.Concurrency, func(ctx context.Context, item barcodeItem) error {
		_, err := s.client.Variations.SetBarcode(ctx, item.plentyItemID, item.plentyVarID, &plenty.CreateVariationBarcodeRequest{
			BarcodeID: item.barcodeConfID,
			Code:      item.code,
		})
		if err != nil {
			rc.Logger.Error("failed to set barcode",
				slog.Int64("variation_id", item.variation.ID),
				slog.String("barcode", item.code),
				slog.Any("error", err),
			)
			return err
		}
		rc.Logger.Debug("set barcode on variation",
			slog.Int64("variation_id", item.variation.ID),
			slog.Int64("plenty_variation_id", item.plentyVarID),
			slog.String("barcode", item.code),
		)
		return nil
	})

	rc.Logger.Info("barcode setting complete",
		slog.Int("succeeded", bcSucceeded),
		slog.Int("failed", bcFailed),
	)

	return nil
}

// ---------------------------------------------------------------------------
// Phase 4: Graduated Prices
// ---------------------------------------------------------------------------

func (s *VariationStage) executeGraduatedPricePhase(ctx context.Context, rc *RunContext, resolved []resolvedVariation) error {
	// Collect graduated prices for all variations from DB.
	type variationGradPrices struct {
		rv     resolvedVariation
		prices []queries.VariationGraduatedPrice
	}
	var withGradPrices []variationGradPrices

	for _, rv := range resolved {
		gps, err := s.db.ListGraduatedPricesByVariation(ctx, rv.item.variation.ID)
		if err != nil {
			rc.Logger.Warn("failed to list graduated prices",
				slog.Int64("variation_id", rv.item.variation.ID),
				slog.Any("error", err),
			)
			continue
		}
		if len(gps) > 0 {
			withGradPrices = append(withGradPrices, variationGradPrices{rv: rv, prices: gps})
		}
	}

	if len(withGradPrices) == 0 {
		rc.Logger.Info("no variations with graduated prices")
		return nil
	}

	// Collect all unique minimum quantities needed across all variations.
	// Also check if any tier has an RRP value set.
	neededQtys := make(map[int]bool)
	hasAnyRRP := false
	for _, vgp := range withGradPrices {
		for _, gp := range vgp.prices {
			neededQtys[int(gp.MinimumQuantity)] = true
			if rrp, err := strconv.ParseFloat(gp.Rrp, 64); err == nil && rrp > 0 {
				hasAnyRRP = true
			}
		}
	}

	// Fetch existing sales price configs and build two maps:
	// qtyToDefaultConfigID: minQty → default (selling price) config ID
	// qtyToRrpConfigID:     minQty → rrp config ID
	// Filter out B2B configs — graduated prices are retail only.
	configs, err := s.client.Variations.ListSalesPriceConfigs(ctx)
	if err != nil {
		return fmt.Errorf("listing sales price configs for graduated pricing: %w", err)
	}

	qtyToDefaultConfigID := make(map[int]int64)
	qtyToRrpConfigID := make(map[int]int64)
	nextPosition := 10
	for _, c := range configs {
		if c.Position >= nextPosition {
			nextPosition = c.Position + 1
		}
		if c.MinimumOrderQuantity <= 0 || isB2BConfig(c) {
			continue // skip base-price configs and B2B configs
		}
		qty := int(c.MinimumOrderQuantity)
		switch c.Type {
		case "default":
			qtyToDefaultConfigID[qty] = c.ID
		case "rrp":
			qtyToRrpConfigID[qty] = c.ID
		}
	}

	// Create missing sales price configs for each needed quantity tier.
	for qty := range neededQtys {
		// Default (selling price) config per tier.
		if _, exists := qtyToDefaultConfigID[qty]; !exists {
			created, cErr := s.client.Variations.CreateSalesPriceConfig(ctx, &plenty.CreateSalesPriceConfigRequest{
				Type:                 "default",
				Position:             nextPosition,
				MinimumOrderQuantity: float64(qty),
			})
			if cErr != nil {
				rc.Logger.Warn("could not create graduated default price config",
					"min_qty", qty,
					"error", cErr,
				)
			} else {
				qtyToDefaultConfigID[qty] = created.ID
				nextPosition++
				rc.Logger.Info("created graduated default sales price config",
					"id", created.ID,
					"min_qty", qty,
				)
			}
		}

		// RRP config per tier (only if any variation has graduated RRP).
		if hasAnyRRP {
			if _, exists := qtyToRrpConfigID[qty]; !exists {
				created, cErr := s.client.Variations.CreateSalesPriceConfig(ctx, &plenty.CreateSalesPriceConfigRequest{
					Type:                 "rrp",
					Position:             nextPosition,
					MinimumOrderQuantity: float64(qty),
				})
				if cErr != nil {
					rc.Logger.Warn("could not create graduated RRP price config",
						"min_qty", qty,
						"error", cErr,
					)
				} else {
					qtyToRrpConfigID[qty] = created.ID
					nextPosition++
					rc.Logger.Info("created graduated RRP sales price config",
						"id", created.ID,
						"min_qty", qty,
					)
				}
			}
		}
	}

	// Build graduated price items: selling price + RRP per tier.
	var gradItems []graduatedPriceItem
	for _, vgp := range withGradPrices {
		for _, gp := range vgp.prices {
			qty := int(gp.MinimumQuantity)

			// Selling price item.
			if configID, ok := qtyToDefaultConfigID[qty]; ok {
				price, parseErr := strconv.ParseFloat(gp.Price, 64)
				if parseErr == nil && price > 0 {
					gradItems = append(gradItems, graduatedPriceItem{
						variation:    vgp.rv.item.variation,
						plentyItemID: vgp.rv.plentyItemID,
						plentyVarID:  vgp.rv.plentyVarID,
						salesPriceID: configID,
						price:        price,
						minQty:       qty,
					})
				}
			}

			// RRP item (if this tier has an RRP value).
			if configID, ok := qtyToRrpConfigID[qty]; ok {
				rrp, parseErr := strconv.ParseFloat(gp.Rrp, 64)
				if parseErr == nil && rrp > 0 {
					gradItems = append(gradItems, graduatedPriceItem{
						variation:    vgp.rv.item.variation,
						plentyItemID: vgp.rv.plentyItemID,
						plentyVarID:  vgp.rv.plentyVarID,
						salesPriceID: configID,
						price:        rrp,
						minQty:       qty,
					})
				}
			}
		}
	}

	if len(gradItems) == 0 {
		rc.Logger.Info("no graduated price items to set")
		return nil
	}

	gradSucceeded, gradFailed := ProcessItems(ctx, gradItems, s.cfg.Concurrency, func(ctx context.Context, item graduatedPriceItem) error {
		_, err := s.client.Variations.SetSalesPrice(ctx, item.plentyItemID, item.plentyVarID, &plenty.VariationSalesPriceRequest{
			SalesPriceID: item.salesPriceID,
			Price:        item.price,
		})
		if err != nil {
			rc.Logger.Error("failed to set graduated price",
				slog.Int64("variation_id", item.variation.ID),
				slog.Int("min_qty", item.minQty),
				slog.Float64("price", item.price),
				slog.Any("error", err),
			)
			return err
		}
		rc.Logger.Debug("set graduated price on variation",
			slog.Int64("variation_id", item.variation.ID),
			slog.Int64("plenty_variation_id", item.plentyVarID),
			slog.Int("min_qty", item.minQty),
			slog.Float64("price", item.price),
		)
		return nil
	})

	rc.Logger.Info("graduated price setting complete",
		slog.Int("succeeded", gradSucceeded),
		slog.Int("failed", gradFailed),
	)

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveDefaultSalesPriceID fetches sales price configs from PlentyONE and
// returns the ID of the default one. Results are cached for subsequent calls.
func (s *VariationStage) resolveDefaultSalesPriceID(ctx context.Context) (int64, error) {
	if s.defaultSalesPriceID != 0 {
		return s.defaultSalesPriceID, nil
	}

	configs, err := s.client.Variations.ListSalesPriceConfigs(ctx)
	if err != nil {
		return 0, err
	}

	if len(configs) == 0 {
		return 0, nil
	}

	// Prefer the config with type "default"; fall back to first.
	for _, c := range configs {
		if c.Type == "default" {
			s.defaultSalesPriceID = c.ID
			return c.ID, nil
		}
	}

	s.defaultSalesPriceID = configs[0].ID
	return configs[0].ID, nil
}

func (s *VariationStage) processVariation(ctx context.Context, rc *RunContext, item variationItem) error {
	check, err := ShouldProcess(ctx, s.db, rc.RunID, item.variation.ID, string(domain.EntityVariation))
	if err != nil {
		return fmt.Errorf("checking variation %d: %w", item.variation.ID, err)
	}
	if !check.NeedsProcessing {
		return nil
	}

	// Resolve the parent item's PlentyONE ID from entity_mappings.
	productMapping, err := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
		RunID:      rc.RunID,
		LocalID:    item.product.ID,
		EntityType: string(domain.EntityProduct),
	})
	if err != nil {
		return s.recordFailure(ctx, rc, item.variation.ID, check.ExistingID,
			fmt.Errorf("resolving PlentyONE item ID for product %d: %w", item.product.ID, err))
	}

	plentyItemID := productMapping.PlentyID

	req := &plenty.CreateVariationRequest{
		Name:   item.variation.Name,
		Number: item.variation.Sku,
	}

	// Resolve attribute values for this variation (Size, Color, etc.).
	varAttrs, vaErr := s.db.ListVariationAttributesByVariation(ctx, item.variation.ID)
	if vaErr != nil {
		rc.Logger.Warn("failed to load variation attributes, creating without attribute links",
			slog.Int64("variation_id", item.variation.ID),
			slog.String("error", vaErr.Error()),
		)
	} else if len(varAttrs) > 0 {
		for _, va := range varAttrs {
			// Resolve PlentyONE attribute ID.
			attrMapping, amErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
				RunID:      rc.RunID,
				LocalID:    va.AttributeID,
				EntityType: string(domain.EntityAttribute),
			})
			if amErr != nil {
				rc.Logger.Warn("failed to resolve PlentyONE attribute ID",
					slog.Int64("local_attr_id", va.AttributeID),
					slog.String("error", amErr.Error()),
				)
				continue
			}
			// Resolve PlentyONE attribute value ID.
			valMapping, vmErr := s.db.GetMappingByLocalIDAndType(ctx, queries.GetMappingByLocalIDAndTypeParams{
				RunID:      rc.RunID,
				LocalID:    va.AttributeValueID,
				EntityType: string(domain.EntityAttributeValue),
			})
			if vmErr != nil {
				rc.Logger.Warn("failed to resolve PlentyONE attribute value ID",
					slog.Int64("local_value_id", va.AttributeValueID),
					slog.String("error", vmErr.Error()),
				)
				continue
			}
			req.VariationAttributeValues = append(req.VariationAttributeValues, plenty.VariationAttributeValue{
				AttributeID: attrMapping.PlentyID,
				ValueID:     valMapping.PlentyID,
			})
		}
	}

	var plentyID int64
	if rc.DryRun {
		plentyID = -1
	} else {
		result, err := s.client.Variations.Create(ctx, plentyItemID, req)
		if err != nil {
			return s.recordFailure(ctx, rc, item.variation.ID, check.ExistingID,
				fmt.Errorf("creating variation in PlentyONE: %w", err))
		}
		plentyID = result.ID
	}

	return s.recordSuccess(ctx, rc, item.variation.ID, plentyID, check.ExistingID)
}

func (s *VariationStage) recordSuccess(ctx context.Context, rc *RunContext, localID, plentyID, existingID int64) error {
	if existingID > 0 {
		// Update existing mapping (retry case).
		return s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusCreated),
			PlentyID:     plentyID,
			ErrorMessage: sql.NullString{},
			ID:           existingID,
		})
	}
	_, err := s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
		RunID:        rc.RunID,
		LocalID:      localID,
		PlentyID:     plentyID,
		EntityType:   string(domain.EntityVariation),
		Stage:        string(domain.StageVariations),
		Status:       string(domain.StatusCreated),
		ErrorMessage: sql.NullString{},
	})
	return err
}

func (s *VariationStage) recordFailure(ctx context.Context, rc *RunContext, localID, existingID int64, originalErr error) error {
	rc.Logger.Error("variation processing failed",
		slog.Int64("local_id", localID),
		slog.Any("error", originalErr),
	)
	if existingID > 0 {
		_ = s.db.UpdateMappingStatus(ctx, queries.UpdateMappingStatusParams{
			Status:       string(domain.StatusFailed),
			PlentyID:     0,
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
			ID:           existingID,
		})
	} else {
		_, _ = s.db.CreateEntityMapping(ctx, queries.CreateEntityMappingParams{
			RunID:        rc.RunID,
			LocalID:      localID,
			PlentyID:     0,
			EntityType:   string(domain.EntityVariation),
			Stage:        string(domain.StageVariations),
			Status:       string(domain.StatusFailed),
			ErrorMessage: sql.NullString{String: originalErr.Error(), Valid: true},
		})
	}
	return originalErr
}

// Ensure VariationStage implements Stage at compile time.
var _ Stage = (*VariationStage)(nil)
