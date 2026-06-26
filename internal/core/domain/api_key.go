package domain

import "time"

type APIKeyStatus string

const (
	KeyStatusActive   APIKeyStatus = "active"
	KeyStatusLimited  APIKeyStatus = "limited"
	KeyStatusCooldown APIKeyStatus = "cooldown"
	KeyStatusDisabled APIKeyStatus = "disabled"
	KeyStatusInvalid  APIKeyStatus = "invalid"
)

type APIKey struct {
	ID          string
	ProviderID  string
	ModelID     string
	Name        string
	Value       string
	ValueEnv    string
	Status      APIKeyStatus
	Priority    int
	ErrorCount  int
	UsedCount   int
	LastUsedAt  *time.Time
	CooldownEnd *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
