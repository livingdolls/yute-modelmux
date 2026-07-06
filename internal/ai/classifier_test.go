package ai

import (
	"testing"
)

func TestClassifierDefaultIsChat(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "chat" {
		t.Fatalf("expected chat, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsCoding(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"write a function that sorts a list"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "coding" {
		t.Fatalf("expected coding, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsReasoning(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"think step by step about why the sky is blue"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "reasoning" {
		t.Fatalf("expected reasoning, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsJSONExtraction(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"extract the name and age from this text and return json"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "json_extraction" {
		t.Fatalf("expected json_extraction, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsSummarization(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"please summarize the following paragraph"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "summarization" {
		t.Fatalf("expected summarization, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsTranslation(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":"translate this to spanish"}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "translation" {
		t.Fatalf("expected translation, got %s", profile.TaskClass)
	}
}

func TestClassifierDetectsToolUse(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"model":"gpt","messages":[],"tools":[{"type":"function","function":{"name":"get_weather"}}]}`)
	profile := c.Classify(body)
	if profile.TaskClass != "tool_use" {
		t.Fatalf("expected tool_use, got %s", profile.TaskClass)
	}
	if !profile.HasToolDefinition {
		t.Fatal("expected HasToolDefinition to be true")
	}
}

func TestClassifierDetectsVision(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"describe this"},{"type":"image_url","image_url":{"url":"http://x"}}]}]}`)
	profile := c.Classify(body)
	if !profile.HasImageContent {
		t.Fatal("expected HasImageContent to be true")
	}
}

func TestClassifierDetectsSystemPrompt(t *testing.T) {
	c := NewClassifier()
	body := []byte(`{"messages":[{"role":"system","content":"you are a helpful assistant"},{"role":"user","content":"hello"}]}`)
	profile := c.Classify(body)
	if !profile.HasSystemPrompt {
		t.Fatal("expected HasSystemPrompt to be true")
	}
}

func TestClassifierDetectsLongContext(t *testing.T) {
	c := NewClassifier()
	large := make([]byte, 9000)
	for i := range large {
		large[i] = 'x'
	}
	profile := c.Classify(large)
	if profile.TaskClass != "long_context" {
		t.Fatalf("expected long_context, got %s", profile.TaskClass)
	}
}
