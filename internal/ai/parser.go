package ai

import "encoding/json"

type ParsedMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ParsedRequest struct {
	Model          string
	SystemPrompt   string
	UserPrompt     string
	AllText        string
	HasTools       bool
	HasFunctions   bool
	ImageCount     int
	Stream         bool
	ResponseFormat string
	RawBody        []byte
}

func ParseRequest(body []byte) *ParsedRequest {
	req := &ParsedRequest{RawBody: body}

	var raw struct {
		Model    string          `json:"model"`
		Messages []ParsedMessage `json:"messages"`
		Stream   *bool           `json:"stream"`
		Tools    json.RawMessage `json:"tools"`
		Functions json.RawMessage `json:"functions"`
		ResponseFormat *struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return req
	}

	req.Model = raw.Model
	req.HasTools = len(raw.Tools) > 2
	req.HasFunctions = len(raw.Functions) > 2
	if raw.Stream != nil {
		req.Stream = *raw.Stream
	}
	if raw.ResponseFormat != nil {
		req.ResponseFormat = raw.ResponseFormat.Type
	}

	var allText []string
	for _, msg := range raw.Messages {
		switch msg.Role {
		case "system":
			text := extractTextContent(msg.Content)
			req.SystemPrompt += text
			allText = append(allText, text)
		case "user":
			text, imgCount := extractUserContent(msg.Content)
			req.UserPrompt += text
			req.ImageCount += imgCount
			allText = append(allText, text)
		default:
			text := extractTextContent(msg.Content)
			allText = append(allText, text)
		}
	}
	for _, t := range allText {
		if t != "" {
			req.AllText += t + " "
		}
	}

	return req
}

func extractTextContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	if content[0] == '"' {
		var s string
		json.Unmarshal(content, &s)
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return ""
	}
	var text string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			text += p.Text + " "
		}
	}
	return text
}

func extractUserContent(content json.RawMessage) (string, int) {
	text := extractTextContent(content)
	imgCount := 0
	if len(content) > 0 && content[0] == '[' {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		json.Unmarshal(content, &parts)
		for _, p := range parts {
			if p.Type == "image_url" {
				imgCount++
			}
		}
	} else if len(content) > 0 {
		var s string
		json.Unmarshal(content, &s)
		text = s
	}
	return text, imgCount
}
