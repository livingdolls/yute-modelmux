package ai

import (
	"regexp"
	"strings"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)sk-[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`(?i)sk-ant-[a-zA-Z0-9_-]{20,}`),
	regexp.MustCompile(`(?i)AIza[0-9A-Za-z_-]{35}`),
	regexp.MustCompile(`(?i)Bearer [a-zA-Z0-9._\-+]{20,}`),
	regexp.MustCompile(`(?i)x-api-key[:\s]+[a-zA-Z0-9._\-+]{20,}`),
	regexp.MustCompile(`(?i)api[-_]?key[:\s]+[a-zA-Z0-9._\-+]{16,}`),
}

type Guardrails struct{}

func NewGuardrails() *Guardrails { return &Guardrails{} }

func (g *Guardrails) Check(cfg config.GuardrailConfig, body []byte) domain.GuardrailResult {
	if !cfg.Enabled {
		return domain.GuardrailResult{Allowed: true, Action: "allow", Reason: "guardrails disabled"}
	}

	if cfg.MaxPromptChars > 0 && len(body) > cfg.MaxPromptChars {
		return domain.GuardrailResult{
			Allowed: false,
			Action:  "block",
			Reason:  "prompt exceeds maximum character limit",
		}
	}

	text := string(body)
	for _, pattern := range secretPatterns {
		if match := pattern.FindString(text); match != "" {
			return domain.GuardrailResult{
				Allowed: false,
				Action:  "block",
				Reason:  "prompt contains secret-like text",
			}
		}
	}

	return domain.GuardrailResult{Allowed: true, Action: "allow", Reason: "passed all checks"}
}

func RedactSecrets(body []byte) []byte {
	text := string(body)
	for _, pattern := range secretPatterns {
		text = pattern.ReplaceAllStringFunc(text, func(match string) string {
			if len(match) > 8 {
				return match[:4] + strings.Repeat("*", len(match)-8) + match[len(match)-4:]
			}
			return "***"
		})
	}
	return []byte(text)
}
