package resource

import "context"

// Source identifies one provider installation.
type Source struct {
	Provider string
	Name     string
	BaseURL  string
}

// Discovered is provider metadata for one resource.
type Discovered struct {
	ProviderID  string
	ExternalKey string
	Name        string
	URL         string
	Metadata    string
}

// Located combines a resource with its workspace-local location, when present.
type Located struct {
	Resource     Resource
	LocationPath string
}

// SyncStore persists provider discoveries and workspace-local locations.
type SyncStore interface {
	UpsertDiscovered(context.Context, Source, string, string, []Discovered) error
	ListByWorkspace(context.Context, string, string, string) ([]Located, error)
	SetLocation(context.Context, string, string, string, string) error
}
