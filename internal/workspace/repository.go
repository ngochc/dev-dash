package workspace

import (
	"context"
	"errors"
)

var (
	ErrNotFound   = errors.New("workspace not found")
	ErrNameExists = errors.New("workspace name already exists")
)

// Repository persists workspaces.
type Repository interface {
	Create(context.Context, Workspace) error
	List(context.Context) ([]Workspace, error)
	GetByID(context.Context, string) (Workspace, error)
	GetByName(context.Context, string) (Workspace, error)
	Delete(context.Context, string) error
}
