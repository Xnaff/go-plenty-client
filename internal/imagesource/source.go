// Package imagesource defines the interface and types for stock photo search providers.
// Each provider (Unsplash, Pexels, Pixabay) implements the ImageSource interface
// with rate limiting and provider-specific attribution formatting.
package imagesource

import "context"

// ImageSource is the interface that stock photo providers must implement.
type ImageSource interface {
	// Search finds photos matching the query with the given options.
	Search(ctx context.Context, query string, opts SearchOptions) ([]Photo, error)

	// Download fetches the image data from the given URL.
	Download(ctx context.Context, downloadURL string) ([]byte, error)

	// Name returns the source provider name (e.g., "unsplash", "pexels", "pixabay").
	Name() string
}

// SearchOptions configures a photo search request.
type SearchOptions struct {
	Page        int    // Page number (1-based)
	PerPage     int    // Results per page
	Orientation string // "landscape", "portrait", or "square"
	MinWidth    int    // Minimum image width in pixels
	MinHeight   int    // Minimum image height in pixels
}

// Photo represents a single stock photo result.
type Photo struct {
	ID           string // Provider-specific photo identifier
	Description  string // Photo description or title
	URL          string // Preview/thumbnail URL
	DownloadURL  string // Full-resolution download URL
	Width        int    // Image width in pixels
	Height       int    // Image height in pixels
	Photographer string // Photographer name
	Attribution  string // Pre-formatted attribution string
	SourceName   string // Provider name: "unsplash", "pexels", "pixabay"
	LicenseType  string // License identifier (e.g., "unsplash", "pexels", "pixabay")
}
