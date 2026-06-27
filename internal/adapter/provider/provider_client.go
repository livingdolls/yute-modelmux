package provider

import (
	"context"
	"net/http"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type ProviderClient interface {
	Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error)
	TestKey(ctx context.Context, provider domain.Provider, apiKey domain.APIKey) error
}

type ClientRegistry struct {
	clients map[domain.ProviderType]ProviderClient
}

func NewClientRegistry() *ClientRegistry {
	r := &ClientRegistry{
		clients: map[domain.ProviderType]ProviderClient{},
	}
	r.clients[domain.ProviderTypeOpenAICompatible] = &OpenAICompatibleClient{}
	r.clients[domain.ProviderTypeCustom] = &OpenAICompatibleClient{}
	r.clients[domain.ProviderTypeAnthropic] = NewAnthropicClient()
	r.clients[domain.ProviderTypeGemini] = NewGeminiClient()
	return r
}

func (r *ClientRegistry) Get(pt domain.ProviderType) ProviderClient {
	if c, ok := r.clients[pt]; ok {
		return c
	}
	return r.clients[domain.ProviderTypeOpenAICompatible]
}

func (r *ClientRegistry) Register(pt domain.ProviderType, client ProviderClient) {
	r.clients[pt] = client
}
