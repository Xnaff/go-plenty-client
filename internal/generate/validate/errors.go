package validate

import "fmt"

// Severity indicates whether a validation issue is blocking or informational.
type Severity int

const (
	// Error indicates a blocking validation failure.
	Error Severity = iota
	// Warning indicates a non-blocking validation issue (logged but not fatal).
	Warning
)

// String returns the severity as a human-readable string.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	default:
		return "unknown"
	}
}

// ValidationError represents a single validation issue found during output validation.
type ValidationError struct {
	Field   string   // The field name that has the issue (e.g., "name", "description")
	Code    string   // Machine-readable code (e.g., "too_short", "too_long", "truncated", "wrong_language", "html_stripped", "invalid_chars")
	Message string   // Human-readable description
	Level   Severity // Error = blocking, Warning = non-blocking
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s (%s)", e.Level, e.Field, e.Message, e.Code)
}

// HasErrors returns true if any validation error has Error severity.
func HasErrors(errs []ValidationError) bool {
	for _, e := range errs {
		if e.Level == Error {
			return true
		}
	}
	return false
}

// ErrorsOnly returns only the validation errors with Error severity.
func ErrorsOnly(errs []ValidationError) []ValidationError {
	var result []ValidationError
	for _, e := range errs {
		if e.Level == Error {
			result = append(result, e)
		}
	}
	return result
}

// WarningsOnly returns only the validation errors with Warning severity.
func WarningsOnly(errs []ValidationError) []ValidationError {
	var result []ValidationError
	for _, e := range errs {
		if e.Level == Warning {
			result = append(result, e)
		}
	}
	return result
}
