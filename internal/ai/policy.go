package ai

import (
	"strings"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type RoutePolicy struct{}

type RouteDecision struct {
	ReroutedID    string
	FallbackGroup string
	RuleIndex     int
	Matched       bool
}

func NewRoutePolicy() *RoutePolicy {
	return &RoutePolicy{}
}

func (rp *RoutePolicy) Evaluate(rules []config.AIRoutingRuleConfig, profile domain.RequestProfile, apiPath string) RouteDecision {
	isChat := apiPath == "/chat/completions"
	isCompletion := apiPath == "/completions"

	for i, rule := range rules {
		if !matchWhen(rule.When, profile, isChat, isCompletion) {
			continue
		}
		if !matchCapabilities(rule.RequireCapability, profile) {
			continue
		}

		decision := RouteDecision{RuleIndex: i, Matched: true}
		if rule.UseModel != "" {
			decision.ReroutedID = rule.UseModel
		} else if rule.UseGroup != "" {
			decision.ReroutedID = rule.UseGroup
		}
		if rule.FallbackGroup != "" {
			decision.FallbackGroup = rule.FallbackGroup
		}
		return decision
	}

	return RouteDecision{}
}

func matchWhen(when config.AIRoutingRuleWhen, profile domain.RequestProfile, isChat, isCompletion bool) bool {
	if when.Task != "" && when.Task != profile.TaskClass {
		return false
	}
	if when.HasTools != nil && *when.HasTools != profile.HasToolDefinition {
		return false
	}
	if when.HasVision != nil && *when.HasVision != profile.HasImageContent {
		return false
	}
	if when.HasStreaming != nil && *when.HasStreaming != profile.IsStreaming {
		return false
	}
	if when.IsChat != nil && *when.IsChat != isChat {
		return false
	}
	if when.IsCompletion != nil && *when.IsCompletion != isCompletion {
		return false
	}
	return true
}

func matchCapabilities(required []string, profile domain.RequestProfile) bool {
	for _, cap := range required {
		switch cap {
		case "tools":
			if !profile.HasToolDefinition {
				return false
			}
		case "vision":
			if !profile.HasImageContent {
				return false
			}
		case "streaming":
			if !profile.IsStreaming {
				return false
			}
		case "json_mode":
			if profile.TaskClass != "json_extraction" {
				return false
			}
		case "chat":
			return true
		case "completions":
			return true
		}
	}
	return true
}

func (rp *RoutePolicy) RuleTraceSummary(decision RouteDecision) string {
	if !decision.Matched {
		return "no matching routing rule"
	}
	parts := []string{"rule matched"}
	if decision.ReroutedID != "" {
		parts = append(parts, "rerouted to "+decision.ReroutedID)
	}
	if decision.FallbackGroup != "" {
		parts = append(parts, "fallback group "+decision.FallbackGroup)
	}
	return strings.Join(parts, "; ")
}
