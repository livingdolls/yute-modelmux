package httpserver

import "github.com/livingdolls/yute-modelmux/internal/core/domain"

type modelListCapabilities struct {
	Chat        bool `json:"chat"`
	Completions bool `json:"completions"`
	Streaming   bool `json:"streaming"`
	Tools       bool `json:"tools"`
	Vision      bool `json:"vision"`
	JSONMode    bool `json:"json_mode"`
}

type modelListItem struct {
	ID                   string                `json:"id"`
	Object               string                `json:"object"`
	OwnedBy              string                `json:"owned_by"`
	ModelMuxType         string                `json:"modelmux_type"`
	Name                 string                `json:"name,omitempty"`
	Description          string                `json:"description,omitempty"`
	Strategy             string                `json:"strategy,omitempty"`
	Capabilities         modelListCapabilities `json:"capabilities"`
	RequiredCapabilities []string              `json:"required_capabilities,omitempty"`
	ContextWindow        int                   `json:"context_window,omitempty"`
	MaxOutputTokens      int                   `json:"max_output_tokens,omitempty"`
	Members              int                   `json:"members,omitempty"`
}

func modelListCapabilitiesFromDomain(capabilities domain.Capabilities) modelListCapabilities {
	return modelListCapabilities{
		Chat: capabilities.Chat, Completions: capabilities.Completions,
		Streaming: capabilities.Streaming, Tools: capabilities.Tools,
		Vision: capabilities.Vision, JSONMode: capabilities.JSONMode,
	}
}

func modelListItemFromModel(model domain.Model) modelListItem {
	return modelListItem{
		ID: model.ID, Object: "model", OwnedBy: model.ProviderID,
		ModelMuxType: "model", Name: model.ID,
		Capabilities: modelListCapabilitiesFromDomain(model.Capabilities),
	}
}

func modelListItemFromGroup(group domain.ModelGroup) modelListItem {
	return modelListItem{
		ID: group.ID, Object: "model", OwnedBy: "modelmux",
		ModelMuxType: "group", Name: group.Name, Description: group.Description,
		Strategy:             string(group.Strategy),
		Capabilities:         modelListCapabilitiesFromDomain(group.Capabilities),
		RequiredCapabilities: append([]string(nil), group.RequiredCapabilities...),
		ContextWindow:        group.ContextWindow, MaxOutputTokens: group.MaxOutputTokens,
		Members: len(group.Members),
	}
}
