package domain

type ProviderType string

const (
	ProviderTypeOpenAICompatible ProviderType = "openai-compatible"
	ProviderTypeAnthropic        ProviderType = "anthropic"
	ProviderTypeGemini           ProviderType = "gemini"
	ProviderTypeCustom           ProviderType = "custom"
)

type AuthType string

const (
	AuthTypeBearer AuthType = "bearer"
	AuthTypeHeader AuthType = "header"
)

type Provider struct {
	ID             string
	Name           string
	Type           ProviderType
	BaseURL        string
	AuthType       AuthType
	AuthHeaderName string
	TimeoutSeconds int
	Enabled        bool
}
