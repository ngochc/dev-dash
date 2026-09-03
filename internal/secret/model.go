package secret

import "time"

// Secret is a stored secret and its metadata.
type Secret struct {
	Key         string
	Value       string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
