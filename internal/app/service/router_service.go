package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	providerclient "github.com/livingdolls/yute-modelmux/internal/adapter/provider"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
)

var ErrNoAvailableKey = errors.New("no available key")

type ProxyError struct {
	HTTPStatus int
	Type       string
	Code       string
	Message    string
}

func (e *ProxyError) Error() string { return e.Message }

func InvalidRequestBodyError(msg string) *ProxyError {
	return &ProxyError{
		HTTPStatus: http.StatusBadRequest,
		Type:       "modelmux_invalid_request",
		Code:       "invalid_request_body",
		Message:    msg,
	}
}

func NotFoundError(msg string) *ProxyError {
	return &ProxyError{
		HTTPStatus: http.StatusNotFound,
		Type:       "modelmux_not_found",
		Code:       "not_found",
		Message:    msg,
	}
}

func DisabledError(msg string) *ProxyError {
	return &ProxyError{
		HTTPStatus: http.StatusForbidden,
		Type:       "modelmux_disabled",
		Code:       "resource_disabled",
		Message:    msg,
	}
}

func AllKeysUnavailableError(modelID string) *ProxyError {
	return &ProxyError{
		HTTPStatus: http.StatusTooManyRequests,
		Type:       "modelmux_all_keys_unavailable",
		Code:       "all_keys_limited",
		Message:    fmt.Sprintf("all API keys for model %s are currently limited or unavailable", modelID),
	}
}

func GroupUnavailableError(groupID string) *ProxyError {
	return &ProxyError{
		HTTPStatus: http.StatusTooManyRequests,
		Type:       "modelmux_group_unavailable",
		Code:       "all_group_models_unavailable",
		Message:    fmt.Sprintf("all models in group %s are currently unavailable", groupID),
	}
}

type RouterService struct {
	cfg          *config.Config
	client       *providerclient.OpenAICompatibleClient
	mu           sync.Mutex
	rrIndex      map[string]int
	groupRRIndex map[string]int
	logs         []domain.RequestLog
	providers    []domain.Provider
	models       []domain.Model
	groups       []domain.ModelGroup
	keys         []domain.APIKey
}

func NewRouterService(cfg *config.Config) *RouterService {
	rs := &RouterService{
		cfg:          cfg,
		client:       providerclient.New(),
		rrIndex:      map[string]int{},
		groupRRIndex: map[string]int{},
	}
	for _, p := range cfg.Providers {
		rs.providers = append(rs.providers, domain.Provider{
			ID:             p.ID,
			Name:           p.Name,
			Type:           domain.ProviderType(p.Type),
			BaseURL:        config.NormalizeBaseURL(p.BaseURL),
			AuthType:       domain.AuthType(p.AuthType),
			AuthHeaderName: p.AuthHeaderName,
			TimeoutSeconds: p.TimeoutSeconds,
			Enabled:        p.Enabled,
		})
	}
	for _, m := range cfg.Models {
		rs.models = append(rs.models, domain.Model{ID: m.ID, ProviderID: m.ProviderID, ModelName: m.ModelName, Strategy: domain.RotationStrategy(m.Strategy), Enabled: m.Enabled})
	}
	for _, g := range cfg.ModelGroups {
		strategy := domain.GroupStrategy(g.Strategy)
		if strategy == "" {
			strategy = domain.GroupStrategyFailover
		}
		group := domain.ModelGroup{ID: g.ID, Name: g.Name, Strategy: strategy, Enabled: g.Enabled}
		for i, member := range g.Members {
			priority := member.Priority
			if priority == 0 {
				priority = i + 1
			}
			weight := member.Weight
			if weight <= 0 {
				weight = 1
			}
			group.Members = append(group.Members, domain.ModelGroupMember{ModelID: member.ModelID, Priority: priority, Weight: weight, Enabled: member.Enabled})
		}
		rs.groups = append(rs.groups, group)
	}
	now := time.Now()
	for _, k := range cfg.Keys {
		status := domain.APIKeyStatus(k.Status)
		if status == "" {
			status = domain.KeyStatusActive
		}
		value := k.Value
		if value == "" && k.ValueEnv != "" {
			value = os.Getenv(k.ValueEnv)
		}
		rs.keys = append(rs.keys, domain.APIKey{ID: k.ID, ProviderID: k.ProviderID, ModelID: k.ModelID, Name: k.Name, Value: value, ValueEnv: k.ValueEnv, Status: status, Priority: k.Priority, CreatedAt: now, UpdatedAt: now})
	}
	return rs
}

func (s *RouterService) ListProviders() []domain.Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Provider(nil), s.providers...)
}
func (s *RouterService) ListModels() []domain.Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Model(nil), s.models...)
}
func (s *RouterService) ListModelGroups() []domain.ModelGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.ModelGroup(nil), s.groups...)
}
func (s *RouterService) ListKeys() []domain.APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.APIKey(nil), s.keys...)
}
func (s *RouterService) Logs() []domain.RequestLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.RequestLog(nil), s.logs...)
}

func (s *RouterService) SelectKey(ctx context.Context, modelID string) (*domain.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	model, ok := s.modelByID(modelID)
	if !ok || !model.Enabled {
		return nil, fmt.Errorf("model %s not found or disabled", modelID)
	}
	keys := s.availableKeys(modelID)
	if len(keys) == 0 {
		return nil, ErrNoAvailableKey
	}

	switch model.Strategy {
	case domain.StrategyRoundRobin:
		idx := s.rrIndex[modelID] % len(keys)
		s.rrIndex[modelID] = (idx + 1) % len(keys)
		k := keys[idx]
		return &k, nil
	case domain.StrategyLeastError:
		sort.SliceStable(keys, func(i, j int) bool {
			if keys[i].ErrorCount != keys[j].ErrorCount {
				return keys[i].ErrorCount < keys[j].ErrorCount
			}
			if keys[i].UsedCount != keys[j].UsedCount {
				return keys[i].UsedCount < keys[j].UsedCount
			}
			return keys[i].Priority < keys[j].Priority
		})
		k := keys[0]
		return &k, nil
	default:
		sort.SliceStable(keys, func(i, j int) bool { return keys[i].Priority < keys[j].Priority })
		k := keys[0]
		return &k, nil
	}
}

func (s *RouterService) HandleChatCompletion(ctx context.Context, req *http.Request) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, InvalidRequestBodyError(err.Error())
	}
	requestedID, err := extractModelFromBody(bodyBytes)
	if err != nil {
		return nil, InvalidRequestBodyError(err.Error())
	}
	model, ok := s.modelByID(requestedID)
	if ok {
		if !model.Enabled {
			return nil, DisabledError(fmt.Sprintf("model %s is disabled", requestedID))
		}
		return s.handleModelChatCompletion(ctx, req, bodyBytes, "", model)
	}

	group, ok := s.groupByID(requestedID)
	if ok {
		if !group.Enabled {
			return nil, DisabledError(fmt.Sprintf("model group %s is disabled", requestedID))
		}
		return s.handleGroupChatCompletion(ctx, req, bodyBytes, group)
	}

	return nil, NotFoundError(fmt.Sprintf("unknown model or model group %s", requestedID))
}

func (s *RouterService) handleGroupChatCompletion(ctx context.Context, req *http.Request, bodyBytes []byte, group domain.ModelGroup) (*http.Response, error) {
	attemptedModels := map[string]struct{}{}

	for len(attemptedModels) < len(group.Members) {
		member, model, ok := s.SelectGroupMember(group.ID, attemptedModels)
		if !ok {
			return nil, GroupUnavailableError(group.ID)
		}
		attemptedModels[member.ModelID] = struct{}{}

		resp, err := s.handleModelChatCompletion(ctx, req, bodyBytes, group.ID, model)
		if isUnavailable(err) {
			continue
		}
		return resp, err
	}

	return nil, GroupUnavailableError(group.ID)
}

func (s *RouterService) handleModelChatCompletion(ctx context.Context, req *http.Request, bodyBytes []byte, groupID string, model domain.Model) (*http.Response, error) {
	provider, ok := s.providerByID(model.ProviderID)
	if !ok || !provider.Enabled {
		return nil, fmt.Errorf("unknown provider %s", model.ProviderID)
	}

	retried := map[string]int{}
	maxRetryPerKey := s.cfg.Retry.MaxRetryPerKey
	maxTotal := s.cfg.Retry.MaxTotalAttempts
	totalKeys := s.keyCountForModel(model.ID)
	if maxTotal <= 0 {
		maxTotal = totalKeys * (maxRetryPerKey + 1)
	}
	if maxTotal == 0 {
		return nil, AllKeysUnavailableError(model.ID)
	}

	skipCount := 0
	for totalAttempts := 0; totalAttempts < maxTotal; {
		key, err := s.SelectKey(ctx, model.ID)
		if err != nil {
			if errors.Is(err, ErrNoAvailableKey) {
				return nil, AllKeysUnavailableError(model.ID)
			}
			return nil, err
		}
		if retried[key.ID] > maxRetryPerKey {
			skipCount++
			if skipCount > totalKeys*2 || skipCount > 100 {
				return nil, AllKeysUnavailableError(model.ID)
			}
			continue
		}
		skipCount = 0
		retried[key.ID]++
		totalAttempts++

		if retried[key.ID] > 1 {
			backoffIdx := retried[key.ID] - 2
			if backoffIdx < len(s.cfg.Retry.BackoffMilliseconds) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(s.cfg.Retry.BackoffMilliseconds[backoffIdx]) * time.Millisecond):
				}
			}
		}

		clonedReq := cloneRequestWithBody(req, bodyBytes)
		startedAt := time.Now()
		resp, err := s.client.ForwardChatCompletion(ctx, provider, model, *key, clonedReq)
		result := classifyResult(resp, err, s.cfg)
		result.ModelID = model.ID
		result.GroupID = groupID
		result.ProviderID = provider.ID
		result.LatencyMs = time.Since(startedAt).Milliseconds()
		_ = s.MarkKeyResult(ctx, key.ID, result)
		if result.Success {
			return resp, nil
		}
		if !result.ShouldRotateKey {
			return resp, err
		}
		if retried[key.ID] <= maxRetryPerKey && totalAttempts < maxTotal {
			s.clearKeyCooldown(key.ID)
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	return nil, AllKeysUnavailableError(model.ID)
}

func (s *RouterService) MarkKeyResult(ctx context.Context, keyID string, result inbound.KeyResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].ID != keyID {
			continue
		}
		now := time.Now()
		s.keys[i].UpdatedAt = now
		s.keys[i].UsedCount++
		s.keys[i].LastUsedAt = &now
		log := domain.RequestLog{ID: fmt.Sprintf("log-%d", now.UnixNano()), GroupID: result.GroupID, ModelID: result.ModelID, ProviderID: result.ProviderID, KeyID: keyID, StatusCode: result.StatusCode, Error: result.Error, LatencyMs: result.LatencyMs, CreatedAt: now}
		if result.Success {
			s.keys[i].ErrorCount = 0
			s.keys[i].Status = domain.KeyStatusActive
			s.keys[i].CooldownEnd = nil
			s.appendLog(log)
			return nil
		}
		s.keys[i].ErrorCount++
		s.keys[i].Status = domain.KeyStatusActive
		if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
			s.keys[i].Status = domain.KeyStatusInvalid
			s.keys[i].CooldownEnd = nil
			s.appendLog(log)
			return nil
		}
		if result.CooldownSeconds > 0 {
			s.keys[i].Status = domain.KeyStatusCooldown
			until := now.Add(time.Duration(result.CooldownSeconds) * time.Second)
			s.keys[i].CooldownEnd = &until
		}
		s.appendLog(log)
		return nil
	}
	return fmt.Errorf("key %s not found", keyID)
}

func (s *RouterService) TestKey(ctx context.Context, keyID string) error {
	s.mu.Lock()
	keyIdx := -1
	for i, k := range s.keys {
		if k.ID == keyID {
			keyIdx = i
			break
		}
	}
	s.mu.Unlock()

	if keyIdx == -1 {
		return fmt.Errorf("key %s not found", keyID)
	}

	s.mu.Lock()
	key := s.keys[keyIdx]
	model, ok := s.modelByID(key.ModelID)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("model %s not found for key %s", key.ModelID, keyID)
	}
	provider, ok := s.providerByID(model.ProviderID)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("provider %s not found for key %s", model.ProviderID, keyID)
	}
	s.mu.Unlock()

	return s.client.TestKey(ctx, provider, key)
}

func (s *RouterService) appendLog(log domain.RequestLog) {
	s.logs = append(s.logs, log)
	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
}

func (s *RouterService) providerByID(id string) (domain.Provider, bool) {
	for _, p := range s.providers {
		if p.ID == id {
			return p, true
		}
	}
	return domain.Provider{}, false
}

func (s *RouterService) modelByID(id string) (domain.Model, bool) {
	for _, m := range s.models {
		if m.ID == id {
			return m, true
		}
	}
	return domain.Model{}, false
}

func (s *RouterService) groupByID(id string) (domain.ModelGroup, bool) {
	for _, g := range s.groups {
		if g.ID == id {
			return g, true
		}
	}
	return domain.ModelGroup{}, false
}

func (s *RouterService) SelectGroupMember(groupID string, attempted map[string]struct{}) (domain.ModelGroupMember, domain.Model, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.groupByID(groupID)
	if !ok || !group.Enabled {
		return domain.ModelGroupMember{}, domain.Model{}, false
	}
	members := s.availableGroupMembers(group, attempted)
	if len(members) == 0 {
		return domain.ModelGroupMember{}, domain.Model{}, false
	}

	switch group.Strategy {
	case domain.GroupStrategyRoundRobin:
		idx := s.groupRRIndex[groupID] % len(members)
		s.groupRRIndex[groupID] = (idx + 1) % len(members)
		return members[idx].member, members[idx].model, true
	case domain.GroupStrategyWeighted:
		m := s.selectWeightedMember(members)
		return m.member, m.model, true
	default:
		sort.SliceStable(members, func(i, j int) bool { return members[i].member.Priority < members[j].member.Priority })
		return members[0].member, members[0].model, true
	}
}

func (s *RouterService) selectWeightedMember(members []availableGroupMember) availableGroupMember {
	totalWeight := 0
	for _, m := range members {
		totalWeight += m.member.Weight
	}
	if totalWeight <= 0 {
		return members[0]
	}
	pick := rand.IntN(totalWeight)
	return selectWeightedMemberByPick(members, pick)
}

func selectWeightedMemberByPick(members []availableGroupMember, pick int) availableGroupMember {
	totalWeight := 0
	for _, m := range members {
		totalWeight += m.member.Weight
	}
	if totalWeight <= 0 {
		return members[0]
	}
	cumulative := 0
	for _, m := range members {
		cumulative += m.member.Weight
		if pick < cumulative {
			return m
		}
	}
	return members[len(members)-1]
}

func (s *RouterService) keyCountForModel(modelID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keyCountForModelLocked(modelID)
}

func (s *RouterService) keyCountForModelLocked(modelID string) int {
	count := 0
	for _, k := range s.keys {
		if k.ModelID == modelID {
			count++
		}
	}
	return count
}

type availableGroupMember struct {
	member domain.ModelGroupMember
	model  domain.Model
}

func (s *RouterService) availableGroupMembers(group domain.ModelGroup, attempted map[string]struct{}) []availableGroupMember {
	var members []availableGroupMember
	for _, member := range group.Members {
		if !member.Enabled {
			continue
		}
		if _, seen := attempted[member.ModelID]; seen {
			continue
		}
		model, ok := s.modelByID(member.ModelID)
		if !ok || !model.Enabled {
			continue
		}
		provider, ok := s.providerByID(model.ProviderID)
		if !ok || !provider.Enabled {
			continue
		}
		if len(s.availableKeys(member.ModelID)) == 0 {
			continue
		}
		members = append(members, availableGroupMember{member: member, model: model})
	}
	return members
}

func (s *RouterService) availableKeys(modelID string) []domain.APIKey {
	now := time.Now()
	var out []domain.APIKey
	for _, k := range s.keys {
		if k.ModelID != modelID {
			continue
		}
		if k.Status == domain.KeyStatusDisabled || k.Status == domain.KeyStatusInvalid {
			continue
		}
		if k.Status == domain.KeyStatusCooldown && k.CooldownEnd != nil && k.CooldownEnd.After(now) {
			continue
		}
		out = append(out, k)
	}
	return out
}

func (s *RouterService) clearKeyCooldown(keyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.keys {
		if s.keys[i].ID == keyID {
			s.keys[i].Status = domain.KeyStatusActive
			s.keys[i].CooldownEnd = nil
			return
		}
	}
}

func cloneRequestWithBody(req *http.Request, body []byte) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.Header = req.Header.Clone()
	return cloned
}

func extractModelFromBody(body []byte) (string, error) {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("invalid JSON request body: %w", err)
	}
	if payload.Model == "" {
		return "", errors.New("request body missing model")
	}
	return payload.Model, nil
}

func classifyResult(resp *http.Response, err error, cfg *config.Config) inbound.KeyResult {
	if err != nil {
		return inbound.KeyResult{ShouldRotateKey: true, Error: err.Error(), CooldownSeconds: cfg.Cooldown.TimeoutSeconds}
	}
	if resp == nil {
		return inbound.KeyResult{ShouldRotateKey: true, Error: "empty response", CooldownSeconds: cfg.Cooldown.TimeoutSeconds}
	}
	result := inbound.KeyResult{StatusCode: resp.StatusCode}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		result.Success = true
	case http.StatusBadRequest:
		result.ShouldRotateKey = false
	case http.StatusUnauthorized, http.StatusForbidden:
		result.ShouldRotateKey = true
	case http.StatusTooManyRequests:
		result.ShouldRotateKey = true
		result.CooldownSeconds = cfg.Cooldown.RateLimitSeconds
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		result.ShouldRotateKey = true
		result.CooldownSeconds = cfg.Cooldown.ServerErrorSeconds
	default:
		if resp.StatusCode >= 500 {
			result.ShouldRotateKey = true
			result.CooldownSeconds = cfg.Cooldown.ServerErrorSeconds
		}
	}
	if !result.Success {
		result.Error = resp.Status
	}
	return result
}

func isUnavailable(err error) bool {
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		return false
	}
	return proxyErr.Code == "all_keys_limited" || proxyErr.Code == "all_group_models_unavailable"
}
