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

type GeminiClient struct{}

func NewGeminiClient() *GeminiClient { return &GeminiClient{} }

func (c *GeminiClient) Forward(ctx context.Context, provider domain.Provider, model domain.Model, apiKey domain.APIKey, req *http.Request, apiPath string) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	geminiReq, isStream, err := convertToGeminiRequest(bodyBytes, model.ModelName)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(provider.BaseURL, "/")
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:%s", baseURL, model.ModelName, geminiAction(isStream))
	if isStream {
		endpoint = endpoint + "?alt=sse"
	}
	if !isStream {
		endpoint = fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, model.ModelName)
	}

	upstreamBody, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, err
	}

	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("x-goog-api-key", apiKey.Value)

	client := newForwardHTTPClient(provider.TimeoutSeconds, isStream)
	resp, err := client.Do(upstream)
	if err != nil {
		return nil, err
	}

	if !isStream {
		return convertGeminiResponse(resp, model.ID)
	}

	reader := convertGeminiStream(resp, model.ID)
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       reader,
	}, nil
}

func (c *GeminiClient) TestKey(ctx context.Context, provider domain.Provider, apiKey domain.APIKey) error {
	baseURL := strings.TrimRight(provider.BaseURL, "/")
	endpoint := fmt.Sprintf("%s/v1beta/models", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-goog-api-key", apiKey.Value)
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

func geminiAction(stream bool) string {
	if stream {
		return "streamGenerateContent"
	}
	return "generateContent"
}

type geminiContent struct {
	Role  string        `json:"role,omitempty"`
	Parts []geminiPart  `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent    `json:"contents"`
	SystemInstruction *geminiContent     `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiGenConfig   `json:"generation_config,omitempty"`
}

type geminiGenConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

func convertToGeminiRequest(body []byte, modelName string) (*geminiRequest, bool, error) {
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

	gr := &geminiRequest{}

	maxTokens := openAIReq.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	gr.GenerationConfig = &geminiGenConfig{
		MaxOutputTokens: maxTokens,
	}
	if openAIReq.Temperature != nil {
		gr.GenerationConfig.Temperature = openAIReq.Temperature
	}
	if openAIReq.TopP != nil {
		gr.GenerationConfig.TopP = openAIReq.TopP
	}
	if openAIReq.Stop != nil {
		switch v := openAIReq.Stop.(type) {
		case string:
			if v != "" {
				gr.GenerationConfig.StopSequences = []string{v}
			}
		case []any:
			for _, s := range v {
				if str, ok := s.(string); ok {
					gr.GenerationConfig.StopSequences = append(gr.GenerationConfig.StopSequences, str)
				}
			}
		}
	}

	var systemText string
	for _, msg := range openAIReq.Messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		content := msgMap["content"]

		if role == "system" {
			if str, ok := content.(string); ok {
				systemText += str
			}
			continue
		}

		geminiRole := "user"
		if role == "assistant" {
			geminiRole = "model"
		}

		text := ""
		if str, ok := content.(string); ok {
			text = str
		}

		gr.Contents = append(gr.Contents, geminiContent{Role: geminiRole, Parts: []geminiPart{{Text: text}}})
	}

	if systemText != "" {
		gr.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: systemText}}}
	}

	return gr, openAIReq.Stream, nil
}

func convertGeminiResponse(resp *http.Response, modelID string) (*http.Response, error) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &http.Response{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	}

	var gr struct {
		Candidates []struct {
			Content struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, err
	}

	if len(gr.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates in response")
	}

	candidate := gr.Candidates[0]
	contentText := ""
	for _, part := range candidate.Content.Parts {
		contentText += part.Text
	}

	finishReason := "stop"
	switch candidate.FinishReason {
	case "MAX_TOKENS":
		finishReason = "length"
	case "SAFETY", "RECITATION":
		finishReason = "content_filter"
	}

	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	if gr.UsageMetadata != nil {
		promptTokens = gr.UsageMetadata.PromptTokenCount
		completionTokens = gr.UsageMetadata.CandidatesTokenCount
		totalTokens = gr.UsageMetadata.TotalTokenCount
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
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
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      totalTokens,
		},
	}

	body, _ := json.Marshal(openAIResp)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func convertGeminiStream(resp *http.Response, modelID string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		created := int(time.Now().Unix())
		openAIID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimPrefix(line, "data: ")
			if line == "" || line == "[DONE]" {
				continue
			}

			var event struct {
				Candidates []struct {
					Content struct {
						Role  string `json:"role"`
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			if len(event.Candidates) == 0 {
				continue
			}

			candidate := event.Candidates[0]
			deltaText := ""
			for _, part := range candidate.Content.Parts {
				deltaText += part.Text
			}

			finishReason := (any)(nil)
			if candidate.FinishReason != "" {
				fr := "stop"
				switch candidate.FinishReason {
				case "MAX_TOKENS":
					fr = "length"
				case "SAFETY", "RECITATION":
					fr = "content_filter"
				}
				finishReason = fr
			}

			delta := map[string]any{}
			if deltaText != "" {
				delta = map[string]any{"content": deltaText}
			}

			chunk := map[string]any{
				"id":      openAIID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   modelID,
				"choices": []map[string]any{{
					"index":         0,
					"delta":         delta,
					"finish_reason": finishReason,
				}},
			}
			jsonChunk, _ := json.Marshal(chunk)
			_, _ = pw.Write([]byte("data: " + string(jsonChunk) + "\n\n"))
		}
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()
	return pr
}
