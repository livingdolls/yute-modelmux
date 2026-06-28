package provider

import (
	"bytes"
	"context"
	"encoding/json"
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

func (c *OpenAICompatibleClient) Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error) {
	return c.forwardRequest(ctx, provider, model, apiKey, req, apiPath)
}

func (c *OpenAICompatibleClient) ForwardChatCompletion(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request) (*http.Response, error) {
	return c.forwardRequest(ctx, provider, model, apiKey, req, "/chat/completions")
}

func (c *OpenAICompatibleClient) ForwardCompletion(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request) (*http.Response, error) {
	return c.forwardRequest(ctx, provider, model, apiKey, req, "/completions")
}

func (c *OpenAICompatibleClient) forwardRequest(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error) {
	endpoint, err := providerEndpoint(provider.BaseURL, apiPath)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	rewrittenBody, err := rewriteModelName(bodyBytes, model.ModelName)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	upstream, err := http.NewRequestWithContext(ctx, req.Method, endpoint, bytes.NewReader(rewrittenBody))
	if err != nil {
		return nil, err
	}
	upstream.Header = req.Header.Clone()
	upstream.Header.Set("Content-Type", "application/json")
	setAuthHeader(upstream.Header, provider, apiKey)
	if model.ModelName != "" {
		upstream.Header.Set("X-ModelMux-Model", model.ID)
	}

	isStream := isStreamRequest(bodyBytes)
	client := newForwardHTTPClient(provider.TimeoutSeconds, isStream)
	resp, err := client.Do(upstream)
	if err != nil || resp == nil || !isStream || resp.Body == nil {
		return resp, err
	}
	resp.Body = newOpenAIStreamUsageReadCloser(resp.Body)
	return resp, nil
}

func (c *OpenAICompatibleClient) TestKey(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey) error {
	endpoint, err := providerEndpoint(provider.BaseURL, "/models")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		if !strings.EqualFold(provider.AuthHeaderName, "Authorization") {
			headers.Del("Authorization")
		}
		headers.Set(provider.AuthHeaderName, apiKey.Value)
		return
	}
	headers.Set("Authorization", "Bearer "+apiKey.Value)
}

func newForwardHTTPClient(timeoutSeconds int, stream bool) *http.Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if !stream {
		return &http.Client{Timeout: timeout}
	}
	if timeout <= 0 {
		return &http.Client{}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	return &http.Client{Transport: transport}
}

func providerEndpoint(baseURL, path string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + path
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func rewriteModelName(body []byte, modelName string) ([]byte, error) {
	if modelName == "" {
		return body, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	modelValue, err := json.Marshal(modelName)
	if err != nil {
		return nil, err
	}
	payload["model"] = modelValue
	return json.Marshal(payload)
}

func isStreamRequest(body []byte) bool {
	var payload struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Stream
}
