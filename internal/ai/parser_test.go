package ai

import "testing"

func TestParseRequestModel(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[]}`)
	req := ParseRequest(body)
	if req.Model != "gpt-4" {
		t.Fatalf("expected gpt-4, got %s", req.Model)
	}
}

func TestParseRequestTools(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[],"tools":[{"type":"function","function":{"name":"get_weather"}}]}`)
	req := ParseRequest(body)
	if !req.HasTools {
		t.Fatal("expected HasTools=true")
	}
}

func TestParseRequestNoTools(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[]}`)
	req := ParseRequest(body)
	if req.HasTools {
		t.Fatal("expected HasTools=false")
	}
}

func TestParseRequestStream(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[],"stream":true}`)
	req := ParseRequest(body)
	if !req.Stream {
		t.Fatal("expected Stream=true")
	}
}

func TestParseRequestResponseFormat(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[],"response_format":{"type":"json_object"}}`)
	req := ParseRequest(body)
	if req.ResponseFormat != "json_object" {
		t.Fatalf("expected json_object, got %s", req.ResponseFormat)
	}
}

func TestParseRequestSystemPrompt(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[{"role":"system","content":"you are helpful"},{"role":"user","content":"hello"}]}`)
	req := ParseRequest(body)
	if req.SystemPrompt != "you are helpful" {
		t.Fatalf("expected system prompt, got %q", req.SystemPrompt)
	}
	if req.UserPrompt != "hello" {
		t.Fatalf("expected user prompt, got %q", req.UserPrompt)
	}
}

func TestParseRequestImageContent(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"http://x"}}]}]}`)
	req := ParseRequest(body)
	if req.ImageCount != 1 {
		t.Fatalf("expected 1 image, got %d", req.ImageCount)
	}
}

func TestParseRequestAllText(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[{"role":"system","content":"be helpful"},{"role":"user","content":"code please"}]}`)
	req := ParseRequest(body)
	if req.AllText == "" {
		t.Fatal("expected non-empty AllText")
	}
}

func TestParseRequestNoMessages(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[]}`)
	req := ParseRequest(body)
	if req.AllText != "" {
		t.Fatalf("expected empty AllText, got %q", req.AllText)
	}
}
