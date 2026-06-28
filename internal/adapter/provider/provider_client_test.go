package provider

import (
	"io"
	"strings"
	"testing"
)

func TestStreamUsageParserOpenAI(t *testing.T) {
	data := strings.Join([]string{
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[],"usage":{"prompt_tokens":17,"completion_tokens":25,"total_tokens":42}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	r := newOpenAIStreamUsageReadCloser(io.NopCloser(strings.NewReader(data)))
	buf := make([]byte, 4096)
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	r.Close()

	tracker := r.(StreamUsageTracker)
	prompt, completion := tracker.StreamUsage().Tokens()
	if prompt != 17 {
		t.Fatalf("expected prompt_tokens=17, got %d", prompt)
	}
	if completion != 25 {
		t.Fatalf("expected completion_tokens=25, got %d", completion)
	}
}

func TestStreamUsageParserGemini(t *testing.T) {
	data := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":null}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`,
		``,
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":8,"totalTokenCount":18}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	r := newOpenAIStreamUsageReadCloser(io.NopCloser(strings.NewReader(data)))
	buf := make([]byte, 4096)
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	r.Close()

	tracker := r.(StreamUsageTracker)
	prompt, completion := tracker.StreamUsage().Tokens()
	if prompt != 10 {
		t.Fatalf("expected promptTokenCount=10, got %d", prompt)
	}
	if completion != 8 {
		t.Fatalf("expected candidatesTokenCount=8, got %d", completion)
	}
}

func TestStreamUsageParserNoUsage(t *testing.T) {
	data := strings.Join([]string{
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	r := newOpenAIStreamUsageReadCloser(io.NopCloser(strings.NewReader(data)))
	buf := make([]byte, 4096)
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	r.Close()

	tracker := r.(StreamUsageTracker)
	prompt, completion := tracker.StreamUsage().Tokens()
	if prompt != 0 || completion != 0 {
		t.Fatalf("expected 0 tokens when no usage in stream, got prompt=%d completion=%d", prompt, completion)
	}
}

func TestStreamUsageParserPartialRead(t *testing.T) {
	data := strings.Join([]string{
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[],"usage":{"prompt_tokens":50,"completion_tokens":30}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	r := newOpenAIStreamUsageReadCloser(io.NopCloser(strings.NewReader(data)))
	buf := make([]byte, 20)
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	r.Close()

	tracker := r.(StreamUsageTracker)
	prompt, completion := tracker.StreamUsage().Tokens()
	if prompt != 50 || completion != 30 {
		t.Fatalf("expected prompt=50 completion=30, got prompt=%d completion=%d", prompt, completion)
	}
}

func TestStreamUsageParserConcurrentReads(t *testing.T) {
	data := strings.Join([]string{
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":200}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	r := newOpenAIStreamUsageReadCloser(io.NopCloser(strings.NewReader(data)))

	done := make(chan [2]int)
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := r.Read(buf)
			if err == io.EOF {
				break
			}
		}
		r.Close()
		tracker := r.(StreamUsageTracker)
		p, c := tracker.StreamUsage().Tokens()
		done <- [2]int{p, c}
	}()

	result := <-done
	if result[0] != 100 || result[1] != 200 {
		t.Fatalf("expected prompt=100 completion=200, got prompt=%d completion=%d", result[0], result[1])
	}
}
