package workspace

import (
	"context"
	"errors"
)

var ErrConfigNotFound = errors.New("workspace config not found")

// ConfigRepository persists provider-neutral workspace configuration.
type ConfigRepository interface {
	Set(context.Context, string, ConfigEntry) error
	Get(context.Context, string, string, string) (ConfigEntry, error)
	List(context.Context, string) ([]ConfigEntry, error)
	ListNamespace(context.Context, string, string) ([]ConfigEntry, error)
	Unset(context.Context, string, string, string) error
	ReplaceAll(context.Context, string, []ConfigEntry) error
	ReplaceUser(context.Context, string, []ConfigEntry) error
}
