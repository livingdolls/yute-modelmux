package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type AnthropicClient struct{}

func NewAnthropicClient() *AnthropicClient { return &AnthropicClient{} }

func (c *AnthropicClient) Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error) {
	if apiPath == "/completions" {
		return nil, fmt.Errorf("anthropic provider does not support /v1/completions; use /v1/chat/completions")
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	anthropicReq, isStream, err := convertToAnthropicRequest(bodyBytes, model.ModelName)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/v1/messages"
	upstreamBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, err
	}

	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("x-api-key", apiKey.Value)
	upstream.Header.Set("anthropic-version", "2023-06-01")

	client := newForwardHTTPClient(provider.TimeoutSeconds, isStream)
	resp, err := client.Do(upstream)
	if err != nil {
		return nil, err
	}

	if !isStream {
		return convertAnthropicResponse(resp, model.ID)
	}

	reader := convertAnthropicStream(resp, model.ID)
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       reader,
	}, nil
}

func (c *AnthropicClient) TestKey(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey) error {
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/v1/messages"
	modelName := model.ModelName
	if modelName == "" {
		modelName = model.ID
	}
	if modelName == "" {
		modelName = "claude-3-haiku-20240307"
	}
	body := map[string]any{
		"model":      modelName,
		"max_tokens": 1,
		"messages":   []map[string]any{{"role": "user", "content": "hi"}},
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey.Value)
	req.Header.Set("anthropic-version", "2023-06-01")
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

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []anthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

func convertToAnthropicRequest(body []byte, modelName string) (*anthropicRequest, bool, error) {
	var openAIReq struct {
		Model       string  `json:"model"`
		Messages    []any   `json:"messages"`
		Stream      bool    `json:"stream"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature *float64 `json:"temperature"`
		TopP        *float64 `json:"top_p"`
		Stop        any     `json:"stop"`
	}
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		return nil, false, fmt.Errorf("invalid request body: %w", err)
	}

	maxTokens := openAIReq.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	ar := &anthropicRequest{
		Model:     modelName,
		MaxTokens: maxTokens,
		Stream:    openAIReq.Stream,
	}

	if openAIReq.Temperature != nil {
		ar.Temperature = openAIReq.Temperature
	}
	if openAIReq.TopP != nil {
		ar.TopP = openAIReq.TopP
	}

	if openAIReq.Stop != nil {
		switch v := openAIReq.Stop.(type) {
		case string:
			if v != "" {
				ar.StopSequences = []string{v}
			}
		case []any:
			for _, s := range v {
				if str, ok := s.(string); ok {
					ar.StopSequences = append(ar.StopSequences, str)
				}
			}
		}
	}

	for _, msg := range openAIReq.Messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		content := msgMap["content"]

		if role == "system" {
			if str, ok := content.(string); ok {
				ar.System = str
			}
			continue
		}

		anthropicRole := "user"
		if role == "assistant" {
			anthropicRole = "assistant"
		}
		ar.Messages = append(ar.Messages, anthropicMessage{Role: anthropicRole, Content: content})
	}

	return ar, openAIReq.Stream, nil
}

func convertAnthropicResponse(resp *http.Response, modelID string) (*http.Response, error) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &http.Response{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	var ar struct {
		ID    string `json:"id"`
		Role  string `json:"role"`
		Model string `json:"model"`
		Stop  string `json:"stop_reason"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, err
	}

	contentText := ""
	for _, c := range ar.Content {
		if c.Type == "text" {
			contentText += c.Text
		}
	}

	finishReason := "stop"
	if ar.Stop == "max_tokens" {
		finishReason = "length"
	} else if ar.Stop == "stop_sequence" {
		finishReason = "stop"
	}

	openAIResp := map[string]any{
		"id":      ar.ID,
		"object":  "chat.completion",
		"created": int(time.Now().Unix()),
		"model":   modelID,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": contentText,
			},
			"finish_reason": finishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     ar.Usage.InputTokens,
			"completion_tokens": ar.Usage.OutputTokens,
			"total_tokens":      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		},
	}

	body, _ := json.Marshal(openAIResp)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func convertAnthropicStream(resp *http.Response, modelID string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		created := int(time.Now().Unix())
		contentBuf := strings.Builder{}
		openAIID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var event struct {
				Type    string `json:"type"`
				Index   int    `json:"index"`
				Content *struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					PartialJSON  string `json:"partial_json"`
				} `json:"content_block"`
				Delta *struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					Thinking string `json:"thinking"`
				} `json:"delta"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			var delta map[string]any
			switch event.Type {
			case "content_block_start":
				if event.Content != nil && event.Content.Text != "" {
					delta = map[string]any{"role": "assistant", "content": ""}
				}
			case "content_block_delta":
				if event.Delta != nil {
					text := event.Delta.Text
					if text == "" {
						text = event.Delta.Thinking
					}
					if text != "" {
						contentBuf.WriteString(text)
						delta = map[string]any{"content": text}
					}
				}
			case "message_delta":
			case "message_stop":
				delta = map[string]any{}
			}

			if delta == nil {
				continue
			}

			chunk := map[string]any{
				"id":      openAIID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   modelID,
				"choices": []map[string]any{{
					"index":         event.Index,
					"delta":         delta,
					"finish_reason": nil,
				}},
			}
			jsonChunk, _ := json.Marshal(chunk)
			_, _ = pw.Write([]byte("data: " + string(jsonChunk) + "\n\n"))
		}

		finishChunk := map[string]any{
			"id":      openAIID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   modelID,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			}},
		}
		jsonChunk, _ := json.Marshal(finishChunk)
		_, _ = pw.Write([]byte("data: " + string(jsonChunk) + "\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()
	return pr
}
