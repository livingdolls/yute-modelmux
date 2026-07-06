package ai

import (
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestGuardrailsAllowWhenDisabled(t *testing.T) {
	g := NewGuardrails()
	result := g.Check(config.GuardrailConfig{Enabled: false}, []byte("any text"))
	if !result.Allowed {
		t.Fatal("expected allowed when guardrails disabled")
	}
}

func TestGuardrailsBlockMaxChars(t *testing.T) {
	g := NewGuardrails()
	body := make([]byte, 1001)
	result := g.Check(config.GuardrailConfig{Enabled: true, MaxPromptChars: 1000}, body)
	if result.Allowed {
		t.Fatal("expected block for exceeding max chars")
	}
}

func TestGuardrailsBlockOpenAIKey(t *testing.T) {
	g := NewGuardrails()
	body := []byte(`{"messages":[{"role":"user","content":"my key is sk-12345678901234567890"}]}`)
	result := g.Check(config.GuardrailConfig{Enabled: true}, body)
	if result.Allowed {
		t.Fatal("expected block for OpenAI key pattern")
	}
}

func TestGuardrailsBlockAnthropicKey(t *testing.T) {
	g := NewGuardrails()
	body := []byte(`my key is sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxx`)
	result := g.Check(config.GuardrailConfig{Enabled: true}, body)
	if result.Allowed {
		t.Fatal("expected block for Anthropic key pattern")
	}
}

func TestGuardrailsBlockGeminiKey(t *testing.T) {
	g := NewGuardrails()
	body := []byte(`AIzaSyDxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`)
	result := g.Check(config.GuardrailConfig{Enabled: true}, body)
	if result.Allowed {
		t.Fatal("expected block for Gemini key pattern")
	}
}

func TestGuardrailsAllowCleanText(t *testing.T) {
	g := NewGuardrails()
	result := g.Check(config.GuardrailConfig{Enabled: true, MaxPromptChars: 10000}, []byte("hello world"))
	if !result.Allowed {
		t.Fatal("expected allow for clean text")
	}
}

func TestRedactSecretsMasksKeys(t *testing.T) {
	body := []byte(`my key is sk-1234567890abcdefghij`)
	redacted := RedactSecrets(body)
	got := string(redacted)
	if got == string(body) {
		t.Fatal("expected redaction, got same text")
	}
	if string(body) == got {
		t.Fatal("key should be masked")
	}
}

func TestRedactSecretsPreservesNonSensitiveText(t *testing.T) {
	body := []byte(`hello world, no secrets here`)
	redacted := RedactSecrets(body)
	if string(redacted) != string(body) {
		t.Fatalf("expected no change for clean text, got %q", string(redacted))
	}
}
