package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type OpenAICompatibleClient struct{}

func New() *OpenAICompatibleClient { return &OpenAICompatibleClient{} }

func (c *OpenAICompatibleClient) ForwardChatCompletion(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request) (*http.Response, error) {
	baseURL, err := url.Parse(strings.TrimRight(provider.BaseURL, "/"))
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse("/chat/completions")
	if err != nil {
		return nil, err
	}
	baseURL = baseURL.ResolveReference(endpoint)

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	upstream, err := http.NewRequestWithContext(ctx, req.Method, baseURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	upstream.Header = req.Header.Clone()
	upstream.Header.Set("Content-Type", "application/json")
	setAuthHeader(upstream.Header, provider, apiKey)
	if model.ModelName != "" {
		upstream.Header.Set("X-ModelMux-Model", model.ID)
	}

	client := &http.Client{Timeout: time.Duration(provider.TimeoutSeconds) * time.Second}
	return client.Do(upstream)
}

func (c *OpenAICompatibleClient) TestKey(ctx context.Context, provider domain.Provider, apiKey domain.APIKey) error {
	baseURL, err := url.Parse(strings.TrimRight(provider.BaseURL, "/"))
	if err != nil {
		return err
	}
	endpoint, _ := url.Parse("/models")
	baseURL = baseURL.ResolveReference(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return err
	}
	setAuthHeader(req.Header, provider, apiKey)
	client := &http.Client{Timeout: time.Duration(provider.TimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("key test failed: %s", resp.Status)
	}
	return nil
}

func setAuthHeader(headers http.Header, provider domain.Provider, apiKey domain.APIKey) {
	if provider.AuthType == domain.AuthTypeHeader && provider.AuthHeaderName != "" {
		headers.Set(provider.AuthHeaderName, apiKey.Value)
		return
	}
	headers.Set("Authorization", "Bearer "+apiKey.Value)
}
