package plenty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestEntityClient creates a Client backed by the given handler. The client
// bypasses the full RoundTripper chain (no auth, no rate limiting, no retries)
// to test entity service logic in isolation.
func newTestEntityClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
		dryRun:     false,
		logger:     testLogger(),
	}
	c.Categories = &CategoryService{client: c}
	c.Attributes = &AttributeService{client: c}
	c.Properties = &PropertyService{client: c}
	c.Items = &ItemService{client: c}
	c.Variations = &VariationService{client: c}
	c.Images = &ImageService{client: c}
	c.Texts = &TextService{client: c}

	return c
}

// newDryRunClient returns a Client with dryRun=true and a handler that
// fails the test if any request is received.
func newDryRunClient(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run client made an HTTP call: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
		dryRun:     true,
		logger:     testLogger(),
	}
	c.Categories = &CategoryService{client: c}
	c.Attributes = &AttributeService{client: c}
	c.Properties = &PropertyService{client: c}
	c.Items = &ItemService{client: c}
	c.Variations = &VariationService{client: c}
	c.Images = &ImageService{client: c}
	c.Texts = &TextService{client: c}

	return c
}

// ---------------------------------------------------------------------------
// Category tests
// ---------------------------------------------------------------------------

func TestCategoryService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/categories", func(w http.ResponseWriter, r *http.Request) {
		var req CreateCategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Type != "item" {
			t.Errorf("expected type 'item', got %q", req.Type)
		}
		if len(req.Details) == 0 {
			t.Error("expected at least one category detail")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Category{
			ID:   42,
			Type: req.Type,
		})
	})

	mux.HandleFunc("GET /rest/categories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Category]{
			Page:        1,
			TotalsCount: 2,
			IsLastPage:  true,
			Entries: []Category{
				{ID: 1, Type: "item"},
				{ID: 2, Type: "item"},
			},
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		cat, err := client.Categories.Create(ctx, &CreateCategoryRequest{
			Type: "item",
			Details: []CategoryDetail{
				{Lang: "en", Name: "Electronics"},
			},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if cat.ID != 42 {
			t.Errorf("expected ID 42, got %d", cat.ID)
		}
	})

	t.Run("Create/DryRun", func(t *testing.T) {
		dryClient := newDryRunClient(t)
		cat, err := dryClient.Categories.Create(ctx, &CreateCategoryRequest{
			Type: "item",
			Details: []CategoryDetail{
				{Lang: "en", Name: "Test"},
			},
		})
		if err != nil {
			t.Fatalf("dry-run Create failed: %v", err)
		}
		if cat.ID != -1 {
			t.Errorf("expected dry-run ID -1, got %d", cat.ID)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := client.Categories.List(ctx, PaginationParams{Page: 1, ItemsPerPage: 50})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(resp.Entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(resp.Entries))
		}
		if !resp.IsLastPage {
			t.Error("expected IsLastPage to be true")
		}
	})
}

// ---------------------------------------------------------------------------
// Attribute tests
// ---------------------------------------------------------------------------

func TestAttributeService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/attributes", func(w http.ResponseWriter, r *http.Request) {
		var req CreateAttributeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.BackendName == "" {
			t.Error("expected non-empty backendName")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Attribute{
			ID:          10,
			BackendName: req.BackendName,
			Position:    req.Position,
		})
	})

	mux.HandleFunc("POST /rest/attributes/1/values", func(w http.ResponseWriter, r *http.Request) {
		var req CreateAttributeValueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AttributeValue{
			ID:          20,
			AttributeID: 1,
			BackendName: req.BackendName,
			Position:    req.Position,
		})
	})

	mux.HandleFunc("GET /rest/attributes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Attribute]{
			Page:        1,
			TotalsCount: 1,
			IsLastPage:  true,
			Entries:     []Attribute{{ID: 10, BackendName: "color"}},
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		attr, err := client.Attributes.Create(ctx, &CreateAttributeRequest{
			BackendName: "color",
			Position:    1,
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if attr.ID != 10 {
			t.Errorf("expected ID 10, got %d", attr.ID)
		}
		if attr.BackendName != "color" {
			t.Errorf("expected backendName 'color', got %q", attr.BackendName)
		}
	})

	t.Run("CreateValue", func(t *testing.T) {
		val, err := client.Attributes.CreateValue(ctx, 1, &CreateAttributeValueRequest{
			BackendName: "red",
			Position:    1,
		})
		if err != nil {
			t.Fatalf("CreateValue failed: %v", err)
		}
		if val.ID != 20 {
			t.Errorf("expected ID 20, got %d", val.ID)
		}
		if val.AttributeID != 1 {
			t.Errorf("expected attributeID 1, got %d", val.AttributeID)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := client.Attributes.List(ctx, PaginationParams{Page: 1, ItemsPerPage: 50})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(resp.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(resp.Entries))
		}
	})
}

// ---------------------------------------------------------------------------
// Property tests
// ---------------------------------------------------------------------------

func TestPropertyService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/properties", func(w http.ResponseWriter, r *http.Request) {
		var req CreatePropertyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Cast == "" {
			t.Error("expected non-empty cast")
		}
		if len(req.Names) == 0 {
			t.Error("expected at least one property name")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Property{
			ID:       30,
			Cast:     req.Cast,
			Position: req.Position,
			Names:    req.Names,
		})
	})

	mux.HandleFunc("POST /rest/properties/relations", func(w http.ResponseWriter, r *http.Request) {
		var req PropertyRelationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.RelationTargetID == 0 {
			t.Error("expected non-zero relationTargetId")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PropertyRelation{
			ID:                     50,
			RelationTargetID:       req.RelationTargetID,
			RelationTypeIdentifier: req.RelationTypeIdentifier,
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		prop, err := client.Properties.Create(ctx, &CreatePropertyRequest{
			Cast:     "text",
			Position: 1,
			Names: []PropertyName{
				{Lang: "en", Name: "Material"},
			},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if prop.ID != 30 {
			t.Errorf("expected ID 30, got %d", prop.ID)
		}
		if prop.Cast != "text" {
			t.Errorf("expected cast 'text', got %q", prop.Cast)
		}
	})

	t.Run("CreateRelation", func(t *testing.T) {
		rel, err := client.Properties.CreateRelation(ctx, &PropertyRelationRequest{
			RelationTargetID:       100,
			RelationTypeIdentifier: "variation",
		})
		if err != nil {
			t.Fatalf("CreateRelation failed: %v", err)
		}
		if rel.ID != 50 {
			t.Errorf("expected ID 50, got %d", rel.ID)
		}
		if rel.RelationTargetID != 100 {
			t.Errorf("expected relationTargetId 100, got %d", rel.RelationTargetID)
		}
	})
}

// ---------------------------------------------------------------------------
// Item tests
// ---------------------------------------------------------------------------

func TestItemService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/items", func(w http.ResponseWriter, r *http.Request) {
		var req CreateItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(req.Variations) == 0 {
			t.Error("expected at least one variation in item creation")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Item{
			ID: 99,
			Variations: []Variation{
				{ID: 101, ItemID: 99},
			},
		})
	})

	mux.HandleFunc("GET /rest/items/99", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Item{
			ID: 99,
			Variations: []Variation{
				{ID: 101, ItemID: 99},
			},
		})
	})

	mux.HandleFunc("GET /rest/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Item]{
			Page:        1,
			TotalsCount: 1,
			IsLastPage:  true,
			Entries:     []Item{{ID: 99}},
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		item, err := client.Items.Create(ctx, &CreateItemRequest{
			Variations: []CreateItemVariation{
				{
					Name: "Test Product",
					VariationDefaultCategory: CreateItemCategory{CategoryID: 42},
				},
			},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if item.ID != 99 {
			t.Errorf("expected ID 99, got %d", item.ID)
		}
		if len(item.Variations) != 1 {
			t.Fatalf("expected 1 variation, got %d", len(item.Variations))
		}
		if item.Variations[0].ID != 101 {
			t.Errorf("expected variation ID 101, got %d", item.Variations[0].ID)
		}
	})

	t.Run("Get", func(t *testing.T) {
		item, err := client.Items.Get(ctx, 99)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if item.ID != 99 {
			t.Errorf("expected ID 99, got %d", item.ID)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := client.Items.List(ctx, PaginationParams{Page: 1, ItemsPerPage: 50})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(resp.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(resp.Entries))
		}
	})
}

// ---------------------------------------------------------------------------
// Variation tests
// ---------------------------------------------------------------------------

func TestVariationService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/items/1/variations", func(w http.ResponseWriter, r *http.Request) {
		var req CreateVariationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Verify variationAttributeValues are present
		if len(req.VariationAttributeValues) == 0 {
			t.Error("expected variationAttributeValues in creation request")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Variation{
			ID:                       200,
			ItemID:                   1,
			VariationAttributeValues: req.VariationAttributeValues,
		})
	})

	mux.HandleFunc("PUT /rest/items/1/variations/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		var req UpdateVariationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Variation{
			ID:     2,
			ItemID: 1,
			Name:   req.Name,
		})
	})

	mux.HandleFunc("GET /rest/items/1/variations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Variation]{
			Page:        1,
			TotalsCount: 1,
			IsLastPage:  true,
			Entries:     []Variation{{ID: 200, ItemID: 1}},
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		v, err := client.Variations.Create(ctx, 1, &CreateVariationRequest{
			Name: "Red / Large",
			VariationAttributeValues: []VariationAttributeValue{
				{AttributeID: 10, ValueID: 20},
				{AttributeID: 11, ValueID: 21},
			},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if v.ID != 200 {
			t.Errorf("expected ID 200, got %d", v.ID)
		}
		if len(v.VariationAttributeValues) != 2 {
			t.Errorf("expected 2 attribute values, got %d", len(v.VariationAttributeValues))
		}
	})

	t.Run("Update", func(t *testing.T) {
		v, err := client.Variations.Update(ctx, 1, 2, &UpdateVariationRequest{
			Name: "Updated Name",
		})
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		if v.ID != 2 {
			t.Errorf("expected ID 2, got %d", v.ID)
		}
		if v.Name != "Updated Name" {
			t.Errorf("expected name 'Updated Name', got %q", v.Name)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := client.Variations.List(ctx, 1, PaginationParams{Page: 1, ItemsPerPage: 50})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(resp.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(resp.Entries))
		}
	})
}

// ---------------------------------------------------------------------------
// Image tests (critical: base64 JSON, NOT multipart)
// ---------------------------------------------------------------------------

func TestImageService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/items/1/images/upload", func(w http.ResponseWriter, r *http.Request) {
		// Verify Content-Type is JSON, not multipart
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		// Check for base64 field (uploadImageData or uploadUrl)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("request body is not valid JSON: %v", err)
		}

		if _, ok := raw["uploadImageData"]; ok {
			// Base64 upload
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Image{ID: 5, ItemID: 1, Position: 1})
			return
		}
		if _, ok := raw["uploadUrl"]; ok {
			// URL upload
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Image{ID: 6, ItemID: 1, Position: 1})
			return
		}
		t.Error("request body contains neither uploadImageData nor uploadUrl")
		w.WriteHeader(http.StatusBadRequest)
	})

	mux.HandleFunc("GET /rest/items/1/images", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Image{
			{ID: 5, ItemID: 1, Position: 1, URL: "https://cdn.example.com/img.jpg"},
		})
	})

	mux.HandleFunc("DELETE /rest/items/1/images/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("UploadBase64", func(t *testing.T) {
		img, err := client.Images.UploadBase64(ctx, 1, &UploadImageBase64Request{
			UploadImageData: "aW1hZ2VkYXRh", // "imagedata" in base64
			Position:        1,
		})
		if err != nil {
			t.Fatalf("UploadBase64 failed: %v", err)
		}
		if img.ID != 5 {
			t.Errorf("expected ID 5, got %d", img.ID)
		}
	})

	t.Run("UploadURL", func(t *testing.T) {
		img, err := client.Images.UploadURL(ctx, 1, &UploadImageURLRequest{
			UploadURL: "https://example.com/image.jpg",
			Position:  1,
		})
		if err != nil {
			t.Fatalf("UploadURL failed: %v", err)
		}
		if img.ID != 6 {
			t.Errorf("expected ID 6, got %d", img.ID)
		}
	})

	t.Run("List", func(t *testing.T) {
		images, err := client.Images.List(ctx, 1)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(images) != 1 {
			t.Errorf("expected 1 image, got %d", len(images))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.Images.Delete(ctx, 1, 5)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Text tests (critical: one language per call, NOT all-at-once)
// ---------------------------------------------------------------------------

func TestTextService(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/items/1/variations/2/descriptions", func(w http.ResponseWriter, r *http.Request) {
		var req CreateDescriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Critical: verify lang is a singular string field, not an array
		if req.Lang == "" {
			t.Error("expected non-empty lang field (single language per call)")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Description{
			ItemID:      1,
			VariationID: 2,
			Lang:        req.Lang,
			Name:        req.Name,
			Description: req.Description,
		})
	})

	mux.HandleFunc("PUT /rest/items/1/variations/2/descriptions/en", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		var req CreateDescriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Description{
			ItemID:      1,
			VariationID: 2,
			Lang:        "en",
			Name:        req.Name,
		})
	})

	mux.HandleFunc("GET /rest/items/1/variations/2/descriptions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Description{
			{ItemID: 1, VariationID: 2, Lang: "en", Name: "English Name"},
			{ItemID: 1, VariationID: 2, Lang: "de", Name: "German Name"},
		})
	})

	client := newTestEntityClient(t, mux)
	ctx := context.Background()

	t.Run("CreateDescription", func(t *testing.T) {
		desc, err := client.Texts.CreateDescription(ctx, 1, 2, &CreateDescriptionRequest{
			Lang:        "en",
			Name:        "Test Product",
			Description: "A great product.",
		})
		if err != nil {
			t.Fatalf("CreateDescription failed: %v", err)
		}
		if desc.Lang != "en" {
			t.Errorf("expected lang 'en', got %q", desc.Lang)
		}
		if desc.Name != "Test Product" {
			t.Errorf("expected name 'Test Product', got %q", desc.Name)
		}
	})

	t.Run("UpdateDescription", func(t *testing.T) {
		desc, err := client.Texts.UpdateDescription(ctx, 1, 2, "en", &CreateDescriptionRequest{
			Lang: "en",
			Name: "Updated Name",
		})
		if err != nil {
			t.Fatalf("UpdateDescription failed: %v", err)
		}
		if desc.Lang != "en" {
			t.Errorf("expected lang 'en', got %q", desc.Lang)
		}
	})

	t.Run("ListDescriptions", func(t *testing.T) {
		descs, err := client.Texts.ListDescriptions(ctx, 1, 2)
		if err != nil {
			t.Fatalf("ListDescriptions failed: %v", err)
		}
		if len(descs) != 2 {
			t.Errorf("expected 2 descriptions, got %d", len(descs))
		}
	})
}

// ---------------------------------------------------------------------------
// Dry-run mode (comprehensive: all entity services, zero HTTP calls)
// ---------------------------------------------------------------------------

func TestDryRunMode(t *testing.T) {
	var httpCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		t.Errorf("dry-run client made HTTP call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		httpClient: srv.Client(),
		baseURL:    srv.URL,
		dryRun:     true,
		logger:     testLogger(),
	}
	c.Categories = &CategoryService{client: c}
	c.Attributes = &AttributeService{client: c}
	c.Properties = &PropertyService{client: c}
	c.Items = &ItemService{client: c}
	c.Variations = &VariationService{client: c}
	c.Images = &ImageService{client: c}
	c.Texts = &TextService{client: c}

	ctx := context.Background()

	type dryRunCase struct {
		name string
		fn   func() (int64, error)
	}

	cases := []dryRunCase{
		{"Categories.Create", func() (int64, error) {
			r, err := c.Categories.Create(ctx, &CreateCategoryRequest{Type: "item", Details: []CategoryDetail{{Lang: "en", Name: "Test"}}})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Attributes.Create", func() (int64, error) {
			r, err := c.Attributes.Create(ctx, &CreateAttributeRequest{BackendName: "color"})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Attributes.CreateValue", func() (int64, error) {
			r, err := c.Attributes.CreateValue(ctx, 1, &CreateAttributeValueRequest{BackendName: "red"})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Properties.Create", func() (int64, error) {
			r, err := c.Properties.Create(ctx, &CreatePropertyRequest{Cast: "text", Names: []PropertyName{{Lang: "en", Name: "Material"}}})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Properties.CreateSelection", func() (int64, error) {
			r, err := c.Properties.CreateSelection(ctx, 1, &CreatePropertySelectionRequest{Names: []PropertyName{{Lang: "en", Name: "Cotton"}}})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Properties.CreateRelation", func() (int64, error) {
			r, err := c.Properties.CreateRelation(ctx, &PropertyRelationRequest{RelationTargetID: 1, RelationTypeIdentifier: "variation"})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Items.Create", func() (int64, error) {
			r, err := c.Items.Create(ctx, &CreateItemRequest{Variations: []CreateItemVariation{{Name: "Test"}}})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Variations.Create", func() (int64, error) {
			r, err := c.Variations.Create(ctx, 1, &CreateVariationRequest{Name: "Test"})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Variations.Update", func() (int64, error) {
			r, err := c.Variations.Update(ctx, 1, 2, &UpdateVariationRequest{Name: "Updated"})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Images.UploadBase64", func() (int64, error) {
			r, err := c.Images.UploadBase64(ctx, 1, &UploadImageBase64Request{UploadImageData: "aW1hZ2U=", Position: 1})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Images.UploadURL", func() (int64, error) {
			r, err := c.Images.UploadURL(ctx, 1, &UploadImageURLRequest{UploadURL: "https://example.com/img.jpg", Position: 1})
			if err != nil {
				return 0, err
			}
			return r.ID, nil
		}},
		{"Images.Delete", func() (int64, error) {
			return 0, c.Images.Delete(ctx, 1, 5)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := tc.fn()
			if err != nil {
				t.Fatalf("%s failed in dry-run: %v", tc.name, err)
			}
			// Create methods should return -1, delete returns 0
			if id != -1 && id != 0 && id != 2 { // 2 is for Variations.Update which returns variationID
				t.Errorf("%s: expected dry-run ID -1 (or 0 for delete, 2 for update), got %d", tc.name, id)
			}
		})
	}

	// Also test TextService separately because it returns Description, not an ID directly
	t.Run("Texts.CreateDescription", func(t *testing.T) {
		desc, err := c.Texts.CreateDescription(ctx, 1, 2, &CreateDescriptionRequest{
			Lang: "en",
			Name: "Test",
		})
		if err != nil {
			t.Fatalf("dry-run CreateDescription failed: %v", err)
		}
		if desc.Lang != "en" {
			t.Errorf("expected lang 'en', got %q", desc.Lang)
		}
	})

	t.Run("Texts.UpdateDescription", func(t *testing.T) {
		desc, err := c.Texts.UpdateDescription(ctx, 1, 2, "de", &CreateDescriptionRequest{
			Lang: "de",
			Name: "Test DE",
		})
		if err != nil {
			t.Fatalf("dry-run UpdateDescription failed: %v", err)
		}
		if desc.Lang != "de" {
			t.Errorf("expected lang 'de', got %q", desc.Lang)
		}
	})

	// Final check: no HTTP calls should have been made
	if calls := httpCalls.Load(); calls != 0 {
		t.Errorf("dry-run mode made %d HTTP calls, expected 0", calls)
	}

	// Verify at least the fmt import is used (prevents compile warnings)
	_ = fmt.Sprintf("total dry-run tests: %d", len(cases)+2)
}
