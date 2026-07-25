package service

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

func (s *RouterService) selectGroupMemberForRequest(groupID string, attempted map[string]struct{}, apiPath string, bodyBytes []byte, stickyKey string) (domain.ModelGroupMember, domain.Model, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.groupByID(groupID)
	if !ok || !group.Enabled {
		return domain.ModelGroupMember{}, domain.Model{}, false
	}

	available := s.availableGroupMembers(group, attempted)
	members := make([]availableGroupMember, 0, len(available))
	for _, candidate := range available {
		if err := s.checkModelCapability(candidate.model, apiPath, bodyBytes); err == nil {
			members = append(members, candidate)
		}
	}
	if len(members) == 0 {
		return domain.ModelGroupMember{}, domain.Model{}, false
	}

	switch group.Strategy {
	case domain.GroupStrategyRoundRobin:
		idx := s.groupRRIndex[groupID] % len(members)
		s.groupRRIndex[groupID] = (idx + 1) % len(members)
		return members[idx].member, members[idx].model, true
	case domain.GroupStrategyWeighted:
		selected := s.selectWeightedMember(members)
		return selected.member, selected.model, true
	case domain.GroupStrategyConsistentHash:
		if stickyKey != "" {
			selected := selectRendezvousMember(members, stickyKey)
			return selected.member, selected.model, true
		}
		fallthrough
	default:
		sortGroupMembersByPriority(members)
		return members[0].member, members[0].model, true
	}
}

func sortGroupMembersByPriority(members []availableGroupMember) {
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].member.Priority != members[j].member.Priority {
			return members[i].member.Priority < members[j].member.Priority
		}
		return groupMemberAttemptKey(members[i].member) < groupMemberAttemptKey(members[j].member)
	})
}

func selectRendezvousMember(members []availableGroupMember, stickyKey string) availableGroupMember {
	selected := members[0]
	selectedKey := groupMemberAttemptKey(selected.member)
	bestScore := rendezvousScore(stickyKey, selectedKey)
	for _, candidate := range members[1:] {
		candidateKey := groupMemberAttemptKey(candidate.member)
		score := rendezvousScore(stickyKey, candidateKey)
		if score > bestScore || (score == bestScore && candidateKey < selectedKey) {
			selected = candidate
			selectedKey = candidateKey
			bestScore = score
		}
	}
	return selected
}

func rendezvousScore(stickyKey, memberKey string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(stickyKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(memberKey))
	return h.Sum64()
}

func groupStickyKey(req *http.Request, bodyBytes []byte) string {
	if req != nil {
		for _, header := range []string{"X-ModelMux-Session-ID", "X-OpenAI-User", "OpenAI-User"} {
			if value := strings.TrimSpace(req.Header.Get(header)); value != "" {
				return value
			}
		}
	}

	var payload struct {
		User     string         `json:"user"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return ""
	}
	if value := strings.TrimSpace(payload.User); value != "" {
		return value
	}
	for _, key := range []string{"session_id", "conversation_id", "thread_id"} {
		if value, ok := payload.Metadata[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func (s *RouterService) checkGroupCapability(group domain.ModelGroup, apiPath string, bodyBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	configuredMembers := 0
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		model, ok := s.modelForGroupMemberLocked(member)
		if !ok || !model.Enabled {
			continue
		}
		configuredMembers++
		if err := s.checkModelCapability(model, apiPath, bodyBytes); err == nil {
			return nil
		}
	}
	if configuredMembers == 0 {
		return nil
	}
	return &ProxyError{
		HTTPStatus: http.StatusBadRequest,
		Type:       "modelmux_unsupported",
		Code:       "group_capability_mismatch",
		Message:    fmt.Sprintf("model group %s has no member compatible with this request", group.ID),
	}
}

func (s *RouterService) modelForGroupMemberLocked(member domain.ModelGroupMember) (domain.Model, bool) {
	modelID := member.ModelID
	if member.KeyID != "" {
		key, ok := s.keyByID(member.KeyID)
		if !ok {
			return domain.Model{}, false
		}
		modelID = key.ModelID
	}
	return s.modelByID(modelID)
}

func (s *RouterService) prepareModelGroups() error {
	for i := range s.groups {
		group := &s.groups[i]
		group.Capabilities = s.groupCapabilitiesLocked(*group)
		if !group.Enabled {
			continue
		}

		resolvedMembers := 0
		for _, member := range group.Members {
			if !member.Enabled {
				continue
			}
			model, ok := s.modelForGroupMemberLocked(member)
			if ok && model.Enabled {
				resolvedMembers++
			}
		}
		if resolvedMembers == 0 {
			return fmt.Errorf("enabled model group %s has no enabled model members", group.ID)
		}
		for _, capability := range group.RequiredCapabilities {
			if !capabilityEnabled(group.Capabilities, capability) {
				return fmt.Errorf("model group %s requires capability %s but not every enabled member supports it", group.ID, capability)
			}
		}
	}
	return nil
}

func (s *RouterService) groupCapabilitiesLocked(group domain.ModelGroup) domain.Capabilities {
	var capabilities domain.Capabilities
	found := false
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		model, ok := s.modelForGroupMemberLocked(member)
		if !ok || !model.Enabled {
			continue
		}
		if !found {
			capabilities = model.Capabilities
			found = true
			continue
		}
		capabilities.Chat = capabilities.Chat && model.Capabilities.Chat
		capabilities.Completions = capabilities.Completions && model.Capabilities.Completions
		capabilities.Streaming = capabilities.Streaming && model.Capabilities.Streaming
		capabilities.Tools = capabilities.Tools && model.Capabilities.Tools
		capabilities.Vision = capabilities.Vision && model.Capabilities.Vision
		capabilities.JSONMode = capabilities.JSONMode && model.Capabilities.JSONMode
	}
	return capabilities
}

func capabilityEnabled(capabilities domain.Capabilities, capability string) bool {
	switch capability {
	case "chat":
		return capabilities.Chat
	case "completions":
		return capabilities.Completions
	case "streaming":
		return capabilities.Streaming
	case "tools":
		return capabilities.Tools
	case "vision":
		return capabilities.Vision
	case "json_mode":
		return capabilities.JSONMode
	default:
		return false
	}
}

func setRoutingResponseHeaders(header http.Header, bodyBytes []byte, groupID, modelID, providerID string, attempt int) {
	if requestedID, err := extractModelFromBody(bodyBytes); err == nil {
		header.Set("X-ModelMux-Requested-Model", requestedID)
	}
	header.Set("X-ModelMux-Selected-Model", modelID)
	header.Set("X-ModelMux-Selected-Provider", providerID)
	if groupID != "" {
		header.Set("X-ModelMux-Group", groupID)
	}
	if attempt > 0 {
		header.Set("X-ModelMux-Attempt", strconv.Itoa(attempt))
	}
}
