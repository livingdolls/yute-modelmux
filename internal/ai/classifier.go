package ai

import (
	"strings"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type Classifier struct{}

func NewClassifier() *Classifier { return &Classifier{} }

func (c *Classifier) Classify(body []byte) domain.RequestProfile {
	return c.ClassifyRequest(ParseRequest(body))
}

func (c *Classifier) ClassifyRequest(req *ParsedRequest) domain.RequestProfile {
	profile := domain.RequestProfile{TaskClass: "chat"}
	profile.PromptSize = len(req.RawBody)

	if profile.PromptSize > 8000 {
		profile.TaskClass = "long_context"
	}

	profile.HasSystemPrompt = req.SystemPrompt != ""
	profile.HasToolDefinition = req.HasTools || req.HasFunctions
	profile.HasImageContent = req.ImageCount > 0
	profile.IsStreaming = req.Stream
	profile.HasJSONMode = req.ResponseFormat == "json_object"

	if profile.HasToolDefinition {
		profile.TaskClass = "tool_use"
		profile.DetectedCaps = append(profile.DetectedCaps, "tools")
	}
	if profile.HasImageContent {
		profile.DetectedCaps = append(profile.DetectedCaps, "vision")
		if profile.TaskClass != "tool_use" {
			profile.TaskClass = "vision"
		}
	}
	if profile.IsStreaming {
		profile.DetectedCaps = append(profile.DetectedCaps, "streaming")
	}

	if profile.TaskClass == "chat" || profile.TaskClass == "long_context" {
		text := strings.ToLower(req.AllText)
		task := c.classifyText(text)
		if task != "chat" {
			profile.TaskClass = task
		}
	}

	return profile
}

func (c *Classifier) detectSystemPrompt(text string) bool {
	return strings.Contains(text, `"role":"system"`) || strings.Contains(text, `"role": "system"`)
}

func (c *Classifier) classifyText(text string) string {
	codeKeywords := []string{"def ", "class ", "function ", "func ", "import ", "package ", "fn ", "impl ", "struct ", "trait ", "let ", "const ", "var ", "defun"}
	for _, kw := range codeKeywords {
		if strings.Contains(text, kw) {
			return "coding"
		}
	}

	reasonWords := []string{"reason step", "think step", "analyze the", "solve the", "break down", "logical", "deduce", "proof", "verify"}
	for _, w := range reasonWords {
		if strings.Contains(text, w) {
			return "reasoning"
		}
	}

	summaryWords := []string{"summarize", "summary", "tl;dr", "key points", "recap", "briefly describe"}
	for _, w := range summaryWords {
		if strings.Contains(text, w) {
			return "summarization"
		}
	}

	translateWords := []string{"translate", "translation", "in french", "in spanish", "in german", "in japanese", "in chinese", "in english", "in arabic"}
	for _, w := range translateWords {
		if strings.Contains(text, w) {
			return "translation"
		}
	}

	jsonWords := []string{"json", "extract the", "structured output", "schema", "parse the", "output as json", "return json"}
	for _, w := range jsonWords {
		if strings.Contains(text, w) {
			return "json_extraction"
		}
	}

	return "chat"
}
