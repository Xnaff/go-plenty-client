package domain

import "time"

// Product represents a parent product in the system.
type Product struct {
	ID          int64        `json:"id"`
	JobID       int64        `json:"job_id"`
	Name        string       `json:"name"`
	ProductType string       `json:"product_type"`
	BaseData    ProductData  `json:"base_data"`
	Status      EntityStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// ProductData holds structured product information.
type ProductData struct {
	SKU         string  `json:"sku"`
	Weight      float64 `json:"weight"`
	WeightUnit  string  `json:"weight_unit"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	Barcode     string  `json:"barcode"`
	CategoryIDs []int64 `json:"category_ids"`
}
