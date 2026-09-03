package repository

import "context"

type Source struct {
	Provider string
	Name     string
	BaseURL  string
}

type Remote struct {
	ProviderID  string
	ExternalKey string
	Name        string
	URL         string
}

type Repository struct {
	ResourceID   string
	ProviderID   string
	ExternalKey  string
	Name         string
	URL          string
	CheckoutPath string
}

type State string

const (
	StateNotCloned State = "not-cloned"
	StateCloned    State = "cloned"
	StateMissing   State = "missing"
	StateInvalid   State = "invalid"
)

type Listed struct {
	Repository Repository
	State      State
}

type CheckoutInspection struct {
	Exists bool
	Valid  bool
}

type CheckoutInspector interface {
	Inspect(context.Context, string, string) (CheckoutInspection, error)
}

type DirectoryManager interface {
	Ensure(string) error
}

type Store interface {
	UpsertDiscovered(context.Context, Source, string, []Remote) error
	ListByWorkspace(context.Context, string) ([]Repository, error)
	SetCheckout(context.Context, string, string, string) error
}

type CloneResult struct {
	Repository string
	Status     string
	Error      error
}
