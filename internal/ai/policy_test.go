package ai

import (
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

func boolPtr(b bool) *bool { return &b }

func TestRoutePolicyMatchesTaskCoding(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "coding"}, UseModel: "deepseek-coder"},
	}
	profile := domain.RequestProfile{TaskClass: "coding"}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if !decision.Matched {
		t.Fatal("expected match for coding task")
	}
	if decision.ReroutedID != "deepseek-coder" {
		t.Fatalf("expected deepseek-coder, got %s", decision.ReroutedID)
	}
}

func TestRoutePolicyNoMatchWrongTask(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "coding"}, UseModel: "deepseek-coder"},
	}
	profile := domain.RequestProfile{TaskClass: "chat"}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if decision.Matched {
		t.Fatal("expected no match for chat task when rule is for coding")
	}
}

func TestRoutePolicyMatchesHasTools(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{HasTools: boolPtr(true)}, UseModel: "gpt-with-tools"},
	}
	profile := domain.RequestProfile{HasToolDefinition: true}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if !decision.Matched {
		t.Fatal("expected match for has_tools=true")
	}
	if decision.ReroutedID != "gpt-with-tools" {
		t.Fatalf("expected gpt-with-tools, got %s", decision.ReroutedID)
	}
}

func TestRoutePolicyNoMatchWhenHasToolsFalse(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{HasTools: boolPtr(false)}, UseModel: "simple-model"},
	}
	profile := domain.RequestProfile{HasToolDefinition: true}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if decision.Matched {
		t.Fatal("expected no match when has_tools=false but request has tools")
	}
}

func TestRoutePolicyMatchesUseGroup(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "vision"}, UseGroup: "vision-models"},
	}
	profile := domain.RequestProfile{TaskClass: "vision", HasImageContent: true}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if !decision.Matched {
		t.Fatal("expected match for vision task")
	}
	if decision.ReroutedID != "vision-models" {
		t.Fatalf("expected vision-models, got %s", decision.ReroutedID)
	}
}

func TestRoutePolicyFallbackGroup(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "json_extraction"}, UseModel: "json-model", FallbackGroup: "general-models"},
	}
	profile := domain.RequestProfile{TaskClass: "json_extraction"}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if decision.FallbackGroup != "general-models" {
		t.Fatalf("expected general-models fallback, got %s", decision.FallbackGroup)
	}
}

func TestRoutePolicyFirstMatchWins(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "coding"}, UseModel: "model-a"},
		{When: config.AIRoutingRuleWhen{Task: "coding"}, UseModel: "model-b"},
	}
	profile := domain.RequestProfile{TaskClass: "coding"}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if decision.ReroutedID != "model-a" {
		t.Fatalf("expected first match 'model-a', got %s", decision.ReroutedID)
	}
	if decision.RuleIndex != 0 {
		t.Fatalf("expected rule index 0, got %d", decision.RuleIndex)
	}
}

func TestRoutePolicyIsChatVsCompletion(t *testing.T) {
	rp := NewRoutePolicy()
	rChat := boolPtr(true)
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{IsChat: rChat}, UseModel: "chat-model"},
	}
	profile := domain.RequestProfile{}

	if !rp.Evaluate(rules, profile, "/chat/completions").Matched {
		t.Fatal("expected match for is_chat on /chat/completions")
	}
	if rp.Evaluate(rules, profile, "/completions").Matched {
		t.Fatal("expected no match for is_chat on /completions")
	}
}

func TestRoutePolicyRequireCapabilityTools(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "tool_use"}, RequireCapability: []string{"tools"}, UseModel: "gpt-tools"},
	}
	profile := domain.RequestProfile{TaskClass: "tool_use", HasToolDefinition: true}
	decision := rp.Evaluate(rules, profile, "/chat/completions")
	if !decision.Matched {
		t.Fatal("expected match when required capability 'tools' is present")
	}

	profileNoTools := domain.RequestProfile{TaskClass: "tool_use", HasToolDefinition: false}
	decision2 := rp.Evaluate(rules, profileNoTools, "/chat/completions")
	if decision2.Matched {
		t.Fatal("expected no match when required capability 'tools' is missing")
	}
}

func TestRoutePolicyRequireJSONMode(t *testing.T) {
	rp := NewRoutePolicy()
	rules := []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "json_extraction"}, RequireCapability: []string{"json_mode"}, UseModel: "json-model"},
	}
	profile := domain.RequestProfile{TaskClass: "json_extraction", HasJSONMode: true}
	if !rp.Evaluate(rules, profile, "/chat/completions").Matched {
		t.Fatal("expected match when json_mode is present")
	}
	profileNoJSON := domain.RequestProfile{TaskClass: "json_extraction", HasJSONMode: false}
	if rp.Evaluate(rules, profileNoJSON, "/chat/completions").Matched {
		t.Fatal("expected no match when json_mode is missing")
	}
}

func TestRoutePolicyChatVsCompletionCapability(t *testing.T) {
	rp := NewRoutePolicy()
	rChat := []config.AIRoutingRuleConfig{
		{RequireCapability: []string{"chat"}, UseModel: "chat-model"},
	}
	rComp := []config.AIRoutingRuleConfig{
		{RequireCapability: []string{"completions"}, UseModel: "compl-model"},
	}
	profile := domain.RequestProfile{}

	if !rp.Evaluate(rChat, profile, "/chat/completions").Matched {
		t.Fatal("expected match for require_capability 'chat' on /chat/completions")
	}
	if rp.Evaluate(rChat, profile, "/completions").Matched {
		t.Fatal("expected no match for require_capability 'chat' on /completions")
	}
	if !rp.Evaluate(rComp, profile, "/completions").Matched {
		t.Fatal("expected match for require_capability 'completions' on /completions")
	}
	if rp.Evaluate(rComp, profile, "/chat/completions").Matched {
		t.Fatal("expected no match for require_capability 'completions' on /chat/completions")
	}
}
