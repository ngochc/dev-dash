package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ngochc/dev-dash/internal/secret"
)

// SecretRepository persists secrets in SQLite.
type SecretRepository struct {
	db *sql.DB
}

func NewSecretRepository(db *sql.DB) *SecretRepository {
	return &SecretRepository{db: db}
}

func (r *SecretRepository) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO secrets (key, value, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	if err != nil {
		return fmt.Errorf("upsert secret %q: %w", key, err)
	}
	return nil
}

func (r *SecretRepository) Get(ctx context.Context, key string) (secret.Secret, error) {
	var item secret.Secret
	if err := r.db.QueryRowContext(ctx, `
		SELECT key, value, COALESCE(description, ''), created_at, updated_at
		FROM secrets
		WHERE key = ?
	`, key).Scan(&item.Key, &item.Value, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return secret.Secret{}, secret.ErrNotFound
		}
		return secret.Secret{}, fmt.Errorf("query secret %q: %w", key, err)
	}
	return item, nil
}

func (r *SecretRepository) List(ctx context.Context) ([]secret.Secret, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT key, COALESCE(description, ''), created_at, updated_at
		FROM secrets
		ORDER BY key
	`)
	if err != nil {
		return nil, fmt.Errorf("query secrets: %w", err)
	}
	defer rows.Close()

	var items []secret.Secret
	for rows.Next() {
		var item secret.Secret
		if err := rows.Scan(&item.Key, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secrets: %w", err)
	}
	return items, nil
}

func (r *SecretRepository) Delete(ctx context.Context, key string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM secrets WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("delete secret %q: %w", key, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted secret count: %w", err)
	}
	if rowsAffected == 0 {
		return secret.ErrNotFound
	}
	return nil
}
