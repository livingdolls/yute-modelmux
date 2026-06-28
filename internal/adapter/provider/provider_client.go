package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type ProviderClient interface {
	Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error)
	TestKey(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey) error
}

type StreamUsage struct {
	mu               sync.Mutex
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
	io      io.ReadCloser
	usage   *StreamUsage
	onRead  func([]byte)
	onClose func()
}

func (t *trackedReadCloser) Read(p []byte) (int, error) {
	n, err := t.io.Read(p)
	if n > 0 && t.onRead != nil {
		t.onRead(p[:n])
	}
	if err == io.EOF {
		t.flush()
	}
	return n, err
}

func (t *trackedReadCloser) Close() error {
	t.flush()
	return t.io.Close()
}

func (t *trackedReadCloser) flush() {
	if t.onClose == nil {
		return
	}
	t.onClose()
	t.onClose = nil
}

func (t *trackedReadCloser) StreamUsage() *StreamUsage {
	return t.usage
}

func newTrackedReadCloser(r io.ReadCloser, u *StreamUsage) io.ReadCloser {
	return &trackedReadCloser{io: r, usage: u}
}

func newOpenAIStreamUsageReadCloser(r io.ReadCloser) io.ReadCloser {
	usage := &StreamUsage{}
	parser := &streamUsageParser{usage: usage}
	return &trackedReadCloser{io: r, usage: usage, onRead: parser.Feed, onClose: parser.Flush}
}

type streamUsageParser struct {
	usage *StreamUsage
	line  []byte
}

func (p *streamUsageParser) Feed(data []byte) {
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			p.line = append(p.line, data...)
			return
		}
		p.line = append(p.line, data[:idx]...)
		p.processLine(p.line)
		p.line = p.line[:0]
		data = data[idx+1:]
	}
}

func (p *streamUsageParser) Flush() {
	if len(p.line) == 0 {
		return
	}
	p.processLine(p.line)
	p.line = p.line[:0]
}

func (p *streamUsageParser) processLine(line []byte) {
	line = bytes.TrimRight(line, "\r")
	text := strings.TrimSpace(string(line))
	if !strings.HasPrefix(text, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}

	var event struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}
	if event.Usage != nil {
		p.usage.Add(event.Usage.PromptTokens, event.Usage.CompletionTokens)
		return
	}
	if event.UsageMetadata != nil {
		p.usage.Add(event.UsageMetadata.PromptTokenCount, event.UsageMetadata.CandidatesTokenCount)
	}
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
