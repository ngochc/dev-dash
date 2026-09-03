package secret

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("secret not found")

// Repository persists secrets.
type Repository interface {
	Set(context.Context, string, string) error
	Get(context.Context, string) (Secret, error)
	List(context.Context) ([]Secret, error)
	Delete(context.Context, string) error
}
