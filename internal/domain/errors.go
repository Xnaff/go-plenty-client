package domain

import "fmt"

// ValidationError represents a validation failure on a specific field.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// NotFoundError indicates that an entity was not found.
type NotFoundError struct {
	EntityType EntityType `json:"entity_type"`
	ID         int64      `json:"id"`
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with ID %d not found", e.EntityType, e.ID)
}

// ConflictError indicates a conflict when creating or updating an entity.
type ConflictError struct {
	EntityType EntityType `json:"entity_type"`
	ID         int64      `json:"id"`
	Reason     string     `json:"reason"`
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict on %s with ID %d: %s", e.EntityType, e.ID, e.Reason)
}
