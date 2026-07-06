package domain

import "time"

type RequestProfile struct {
	TaskClass         string
	DetectedCaps      []string
	PromptSize        int
	LanguageHints     []string
	RiskFlags         []string
	IsStreaming       bool
	HasSystemPrompt   bool
	HasToolDefinition bool
	HasImageContent   bool
}

type GuardrailResult struct {
	Allowed bool
	Action  string
	Reason  string
}

type RouteTrace struct {
	ID            string
	RequestID     string
	OriginalModel string
	ReroutedModel string
	Steps         []Step
	CreatedAt     time.Time
}

type Step struct {
	Stage       string
	Decision    string
	Reason      string
	Detail      string
	LatencyMs   int64
}
