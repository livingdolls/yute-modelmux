package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type ProviderClient interface {
	Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error)
	TestKey(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey) error
}

type StreamUsage struct {
	mu           sync.Mutex
	promptTokens     int
	completionTokens int
}

func (u *StreamUsage) Add(prompt, completion int) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.promptTokens = prompt
	u.completionTokens = completion
}

func (u *StreamUsage) Tokens() (int, int) {
	if u == nil {
		return 0, 0
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.promptTokens, u.completionTokens
}

type StreamUsageTracker interface {
	StreamUsage() *StreamUsage
}

type trackedReadCloser struct {
	io   io.ReadCloser
	usage *StreamUsage
}

func (t *trackedReadCloser) Read(p []byte) (int, error) {
	return t.io.Read(p)
}

func (t *trackedReadCloser) Close() error {
	return t.io.Close()
}

func (t *trackedReadCloser) StreamUsage() *StreamUsage {
	return t.usage
}

func newTrackedReadCloser(r io.ReadCloser, u *StreamUsage) io.ReadCloser {
	return &trackedReadCloser{io: r, usage: u}
}

type unsupportedProviderClient struct{}

func (c *unsupportedProviderClient) Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error) {
	return nil, fmt.Errorf("unsupported provider type %q for provider %s", provider.Type, provider.ID)
}

func (c *unsupportedProviderClient) TestKey(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey) error {
	return fmt.Errorf("unsupported provider type %q for provider %s", provider.Type, provider.ID)
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
	return &unsupportedProviderClient{}
}

func (r *ClientRegistry) Register(pt domain.ProviderType, client ProviderClient) {
	r.clients[pt] = client
}
