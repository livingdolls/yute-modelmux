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
	"strings"
	"sync"
	"time"

	providerclient "github.com/livingdolls/yute-modelmux/internal/adapter/provider"
	"github.com/livingdolls/yute-modelmux/internal/ai"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
	"github.com/livingdolls/yute-modelmux/internal/secret"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

type ctxKey int

const (
	ctxKeyStreamResult ctxKey = iota
	ctxKeyTokenTracker
	ctxKeyRequestID
	ctxKeyTraceID
)

func SetRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID).(string)
	return id
}

func setTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTraceID, id)
}

func GetTraceID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyTraceID).(string)
	return id
}

type streamResultInfo struct {
	KeyID      string
	ModelID    string
	ProviderID string
	GroupID    string
	StatusCode int
	Error      string
	StartedAt  time.Time
	ModelIdx   int
}

func SetStreamResultContext(ctx context.Context, info streamResultInfo) context.Context {
	return context.WithValue(ctx, ctxKeyStreamResult, info)
}

func getStreamResultContext(ctx context.Context) (streamResultInfo, bool) {
	info, ok := ctx.Value(ctxKeyStreamResult).(streamResultInfo)
	return info, ok
}

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
	clientReg    *providerclient.ClientRegistry
	store        storage.Storage
	secretStore  *secret.Store
	mu           sync.Mutex
	rrIndex      map[string]int
	groupRRIndex map[string]int
	logs         []domain.RequestLog
	providers    []domain.Provider
	models       []domain.Model
	groups       []domain.ModelGroup
	keys         []domain.APIKey
	aiClassifier *ai.Classifier
	aiGuardrails *ai.Guardrails
	aiTracer     *ai.RouteTracer
	aiPolicy     *ai.RoutePolicy
}

func NewRouterService(cfg *config.Config) (*RouterService, error) {
	return newRouterService(cfg, nil, nil)
}

func NewRouterServiceWithStorage(cfg *config.Config, store storage.Storage) (*RouterService, error) {
	return newRouterService(cfg, store, nil)
}

func NewRouterServiceWithSecret(cfg *config.Config, store storage.Storage, secretStore *secret.Store) (*RouterService, error) {
	return newRouterService(cfg, store, secretStore)
}

func newRouterService(cfg *config.Config, store storage.Storage, secretStore *secret.Store) (*RouterService, error) {
	rs := &RouterService{
		cfg:          cfg,
		clientReg:    providerclient.NewClientRegistry(),
		store:        store,
		secretStore:  secretStore,
		rrIndex:      map[string]int{},
		groupRRIndex: map[string]int{},
		aiClassifier: ai.NewClassifier(),
		aiGuardrails: ai.NewGuardrails(),
		aiTracer:     ai.NewRouteTracer(),
		aiPolicy:     ai.NewRoutePolicy(),
	}
	providerTypes := map[string]domain.ProviderType{}
	for _, p := range cfg.Providers {
		providerTypes[p.ID] = domain.ProviderType(p.Type)
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
		caps := defaultCapabilitiesForType(providerTypes[m.ProviderID], m.Capabilities)
		rs.models = append(rs.models, domain.Model{ID: m.ID, ProviderID: m.ProviderID, ModelName: m.ModelName, Strategy: domain.RotationStrategy(m.Strategy), Enabled: m.Enabled, RequestsPerMinute: m.RequestsPerMinute, MaxConcurrentRequests: m.MaxConcurrentRequests, Capabilities: caps})
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
			group.Members = append(group.Members, domain.ModelGroupMember{ModelID: member.ModelID, KeyID: member.KeyID, Priority: priority, Weight: weight, Enabled: member.Enabled})
		}
		rs.groups = append(rs.groups, group)
	}
	now := time.Now()

	runtimeMap := map[string]storage.KeyRuntimeRecord{}
	if store != nil {
		records, err := store.LoadKeyRuntime()
		if err == nil {
			for _, r := range records {
				runtimeMap[r.KeyID] = r
			}
		}
	}

	for _, k := range cfg.Keys {
		status := domain.APIKeyStatus(k.Status)
		if status == "" {
			status = domain.KeyStatusActive
		}
		value := ""
		if k.SecretRef != "" && secretStore != nil {
			if sv, err := secretStore.Get(k.SecretRef); err == nil {
				value = sv
			}
		}
		if value == "" && k.ValueEnv != "" {
			value = os.Getenv(k.ValueEnv)
		}
		if value == "" {
			value = k.Value
		}
		if value == "" && k.SecretRef != "" {
			if secretStore == nil {
				return nil, fmt.Errorf("key %s uses secret_ref but secret store is not available (set MODELMUX_MASTER_KEY)", k.ID)
			}
			return nil, fmt.Errorf("key %s: secret_ref %q not found in secret store", k.ID, k.SecretRef)
		}
		key := domain.APIKey{
			ID:                    k.ID,
			ProviderID:            k.ProviderID,
			ModelID:               k.ModelID,
			Name:                  k.Name,
			Value:                 value,
			ValueEnv:              k.ValueEnv,
			Status:                status,
			Priority:              k.Priority,
			DailyRequestLimit:     k.DailyRequestLimit,
			DailyTokenLimit:       k.DailyTokenLimit,
			RequestsPerMinute:     k.RequestsPerMinute,
			TokensPerMinute:       k.TokensPerMinute,
			MaxConcurrentRequests: k.MaxConcurrentRequests,
			CreatedAt:             now, UpdatedAt: now,
		}
		if rt, ok := runtimeMap[k.ID]; ok {
			if rt.Status != "" {
				key.Status = domain.APIKeyStatus(rt.Status)
			}
			key.UsedCount = rt.UsedCount
			key.ErrorCount = rt.ErrorCount
			key.DailyRequestCount = rt.DailyRequestCount
			key.DailyTokenCount = rt.DailyTokenCount
			key.DailyDate = rt.DailyDate
			if rt.LastUsedAt != "" {
				if t, err := time.Parse(time.RFC3339, rt.LastUsedAt); err == nil {
					key.LastUsedAt = &t
				}
			}
			if rt.UpdatedAt != "" {
				if t, err := time.Parse(time.RFC3339, rt.UpdatedAt); err == nil {
					key.UpdatedAt = t
				}
			}
			if rt.CooldownEnd != "" {
				cooldownEnd, err := time.Parse(time.RFC3339, rt.CooldownEnd)
				if err == nil {
					key.CooldownEnd = &cooldownEnd
				}
			}
		}
		rs.keys = append(rs.keys, key)
	}

	todayStr := time.Now().Format("2006-01-02")
	for i := range rs.keys {
		if rs.keys[i].DailyDate != todayStr {
			rs.keys[i].DailyRequestCount = 0
			rs.keys[i].DailyTokenCount = 0
			rs.keys[i].DailyDate = todayStr
			if rs.keys[i].Status == domain.KeyStatusLimited {
				rs.keys[i].Status = domain.KeyStatusActive
			}
			rs.persistKeyRuntime(rs.keyRuntimeRecord(rs.keys[i]))
		}
	}

	if store != nil {
		logRecords, err := store.LoadRequestLogs()
		if err == nil {
			for _, lr := range logRecords {
				createdAt := now
				if lr.CreatedAt != "" {
					if t, err := time.Parse(time.RFC3339, lr.CreatedAt); err == nil {
						createdAt = t
					}
				}
				rs.logs = append(rs.logs, domain.RequestLog{
					ID:          lr.ID,
					GroupID:     lr.GroupID,
					ModelID:     lr.ModelID,
					ProviderID:  lr.ProviderID,
					KeyID:       lr.KeyID,
					StatusCode:  lr.StatusCode,
					Error:       lr.Error,
					LatencyMs:   lr.LatencyMs,
					TokenInput:  lr.TokenInput,
					TokenOutput: lr.TokenOutput,
					CreatedAt:   createdAt,
				})
			}
		}
	}

	return rs, nil
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

func (s *RouterService) LogsForMetrics() []domain.RequestLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store != nil {
		records, _, err := s.store.QueryRequestLogs(storage.LogFilter{Limit: 10000})
		if err == nil {
			logs := make([]domain.RequestLog, len(records))
			for i, r := range records {
				createdAt := time.Now()
				if r.CreatedAt != "" {
					if t, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
						createdAt = t
					}
				}
				logs[i] = domain.RequestLog{
					ID: r.ID, GroupID: r.GroupID, ModelID: r.ModelID,
					ProviderID: r.ProviderID, KeyID: r.KeyID,
					StatusCode: r.StatusCode, Error: r.Error,
					LatencyMs: r.LatencyMs, TokenInput: r.TokenInput,
					TokenOutput: r.TokenOutput, CreatedAt: createdAt,
				}
			}
			return logs
		}
	}
	return append([]domain.RequestLog(nil), s.logs...)
}

func (s *RouterService) QueryLogs(filter storage.LogFilter) ([]domain.RequestLog, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store != nil {
		records, total, err := s.store.QueryRequestLogs(filter)
		if err == nil {
			logs := make([]domain.RequestLog, len(records))
			for i, r := range records {
				createdAt := time.Now()
				if r.CreatedAt != "" {
					if t, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
						createdAt = t
					}
				}
				logs[i] = domain.RequestLog{
					ID:          r.ID,
					GroupID:     r.GroupID,
					ModelID:     r.ModelID,
					ProviderID:  r.ProviderID,
					KeyID:       r.KeyID,
					StatusCode:  r.StatusCode,
					Error:       r.Error,
					LatencyMs:   r.LatencyMs,
					TokenInput:  r.TokenInput,
					TokenOutput: r.TokenOutput,
					CreatedAt:   createdAt,
				}
			}
			return logs, total
		}
	}

	var filtered []domain.RequestLog
	for _, log := range s.logs {
		if filter.ModelID != "" && log.ModelID != filter.ModelID {
			continue
		}
		if filter.KeyID != "" && log.KeyID != filter.KeyID {
			continue
		}
		if filter.ProviderID != "" && log.ProviderID != filter.ProviderID {
			continue
		}
		if filter.GroupID != "" && log.GroupID != filter.GroupID {
			continue
		}
		if filter.StatusCode > 0 && log.StatusCode != filter.StatusCode {
			continue
		}
		filtered = append(filtered, log)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	offset := filter.Offset
	if offset > total {
		offset = total
	}
	end := offset + filter.Limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total
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

	var selected domain.APIKey
	switch model.Strategy {
	case domain.StrategyRoundRobin:
		idx := s.rrIndex[modelID] % len(keys)
		s.rrIndex[modelID] = (idx + 1) % len(keys)
		selected = keys[idx]
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
		selected = keys[0]
	case domain.StrategyLeastUsed:
		sort.SliceStable(keys, func(i, j int) bool {
			if keys[i].DailyRequestCount != keys[j].DailyRequestCount {
				return keys[i].DailyRequestCount < keys[j].DailyRequestCount
			}
			return keys[i].Priority < keys[j].Priority
		})
		selected = keys[0]
	default:
		sort.SliceStable(keys, func(i, j int) bool { return keys[i].Priority < keys[j].Priority })
		selected = keys[0]
	}
	k := selected
	return &k, nil
}

func (s *RouterService) selectRequestKey(ctx context.Context, modelID string, requiredKeyID string) (*domain.APIKey, error) {
	if requiredKeyID == "" {
		return s.SelectKey(ctx, modelID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	model, ok := s.modelByID(modelID)
	if !ok || !model.Enabled {
		return nil, fmt.Errorf("model %s not found or disabled", modelID)
	}
	for _, key := range s.availableKeys(modelID) {
		if key.ID == requiredKeyID {
			k := key
			return &k, nil
		}
	}
	return nil, ErrNoAvailableKey
}

func (s *RouterService) HandleChatCompletion(ctx context.Context, req *http.Request) (*http.Response, error) {
	return s.handleOpenAIRequest(ctx, req, "/chat/completions")
}

func (s *RouterService) HandleCompletion(ctx context.Context, req *http.Request) (*http.Response, error) {
	return s.handleOpenAIRequest(ctx, req, "/completions")
}

func (s *RouterService) handleOpenAIRequest(ctx context.Context, req *http.Request, apiPath string) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, InvalidRequestBodyError(err.Error())
	}
	requestedID, err := extractModelFromBody(bodyBytes)
	if err != nil {
		return nil, InvalidRequestBodyError(err.Error())
	}

	var traceID string
	reroutedID := requestedID
	var fallbackGroup string

	if s.cfg.AI.Enabled {
		traceEnabled := s.cfg.AI.RouteTrace.Enabled
		classifierEnabled := s.cfg.AI.Classifier.Enabled
		guardrailsEnabled := s.cfg.AI.Guardrails.Enabled
		hasRoutingRules := len(s.cfg.AI.RoutingRules) > 0

		if traceEnabled || classifierEnabled || guardrailsEnabled || hasRoutingRules {
			requestID := GetRequestID(ctx)
			var trace *domain.RouteTrace
			if traceEnabled {
				trace = s.aiTracer.StartTrace(requestID, requestedID)
				traceID = trace.ID
			}

			var profile domain.RequestProfile

			if classifierEnabled || hasRoutingRules {
				profile = s.aiClassifier.Classify(bodyBytes)
				if trace != nil {
					s.aiTracer.AddStep(traceID, "classifier", profile.TaskClass, "heuristic match", "")
				}
			}

			if guardrailsEnabled {
				result := s.aiGuardrails.Check(s.cfg.AI.Guardrails, bodyBytes)
				if trace != nil {
					s.aiTracer.AddStep(traceID, "guardrails", result.Action, result.Reason, "")
				}
				if !result.Allowed {
					if trace != nil {
						s.aiTracer.FinalizeTrace(traceID, "")
						s.saveTraceIfEnabled(traceID)
					}
					if traceID != "" && s.cfg.AI.RouteTrace.IncludeResponseHeader {
						ctx = setTraceID(ctx, traceID)
						*req = *req.WithContext(ctx)
					}
					return nil, &ProxyError{
						HTTPStatus: http.StatusBadRequest,
						Type:       "modelmux_guardrail_blocked",
						Code:       "guardrail_" + result.Action,
						Message:    result.Reason,
					}
				}
			}

			if hasRoutingRules {
				decision := s.aiPolicy.Evaluate(s.cfg.AI.RoutingRules, profile, apiPath)
				if decision.Matched {
					reroutedID = decision.ReroutedID
					fallbackGroup = decision.FallbackGroup
					if trace != nil {
						s.aiTracer.AddStep(traceID, "routing", decision.ReroutedID,
							s.aiPolicy.RuleTraceSummary(decision), "")
					}
				}
			}

			if trace != nil {
				s.aiTracer.FinalizeTrace(traceID, reroutedID)
				s.saveTraceIfEnabled(traceID)
			}
		}
	}

	model, ok := s.modelByID(reroutedID)
	if ok {
		if !model.Enabled {
			return nil, DisabledError(fmt.Sprintf("model %s is disabled", reroutedID))
		}
		if traceID != "" && s.cfg.AI.RouteTrace.IncludeResponseHeader {
			ctx = setTraceID(ctx, traceID)
			*req = *req.WithContext(ctx)
		}
		return s.handleModelRequest(ctx, req, bodyBytes, "", model, apiPath, "")
	}

	group, ok := s.groupByID(reroutedID)
	if ok {
		if !group.Enabled {
			return nil, DisabledError(fmt.Sprintf("model group %s is disabled", reroutedID))
		}
		if traceID != "" && s.cfg.AI.RouteTrace.IncludeResponseHeader {
			ctx = setTraceID(ctx, traceID)
			*req = *req.WithContext(ctx)
		}
		return s.handleGroupRequest(ctx, req, bodyBytes, group, apiPath)
	}

	if fallbackGroup != "" {
		group, ok := s.groupByID(fallbackGroup)
		if ok && group.Enabled {
			if traceID != "" {
				s.aiTracer.FinalizeTrace(traceID, fallbackGroup)
				s.saveTraceIfEnabled(traceID)
			}
			if traceID != "" && s.cfg.AI.RouteTrace.IncludeResponseHeader {
				ctx = setTraceID(ctx, traceID)
				*req = *req.WithContext(ctx)
			}
			return s.handleGroupRequest(ctx, req, bodyBytes, group, apiPath)
		}
	}

	return nil, NotFoundError(fmt.Sprintf("unknown model or model group %s", reroutedID))
}

func (s *RouterService) handleGroupRequest(ctx context.Context, req *http.Request, bodyBytes []byte, group domain.ModelGroup, apiPath string) (*http.Response, error) {
	attemptedMembers := map[string]struct{}{}

	for len(attemptedMembers) < len(group.Members) {
		member, model, ok := s.SelectGroupMember(group.ID, attemptedMembers)
		if !ok {
			return nil, GroupUnavailableError(group.ID)
		}
		attemptedMembers[groupMemberAttemptKey(member)] = struct{}{}

		resp, err := s.handleModelRequest(ctx, req, bodyBytes, group.ID, model, apiPath, member.KeyID)
		if isUnavailable(err) {
			continue
		}
		return resp, err
	}

	return nil, GroupUnavailableError(group.ID)
}

func (s *RouterService) handleModelRequest(ctx context.Context, req *http.Request, bodyBytes []byte, groupID string, model domain.Model, apiPath string, requiredKeyID string) (*http.Response, error) {
	provider, ok := s.providerByID(model.ProviderID)
	if !ok || !provider.Enabled {
		return nil, fmt.Errorf("unknown provider %s", model.ProviderID)
	}

	if err := s.checkModelCapability(model, apiPath, bodyBytes); err != nil {
		return nil, err
	}

	s.mu.Lock()
	modelIdx := s.modelIndexByID(model.ID)
	if modelIdx >= 0 {
		if s.models[modelIdx].RequestsPerMinute > 0 {
			now := time.Now()
			m := &s.models[modelIdx]
			if m.MinuteWindowStart.IsZero() || now.Sub(m.MinuteWindowStart) >= time.Minute {
				m.MinuteWindowStart = now.Truncate(time.Minute)
				m.MinuteRequestCount = 0
			}
			if m.MinuteRequestCount >= m.RequestsPerMinute {
				s.mu.Unlock()
				return nil, &ProxyError{
					HTTPStatus: http.StatusTooManyRequests,
					Type:       "modelmux_rate_limited",
					Code:       "model_rpm_exceeded",
					Message:    fmt.Sprintf("model %s has exceeded its requests per minute limit", model.ID),
				}
			}
		}
	}
	if modelIdx >= 0 && s.models[modelIdx].MaxConcurrentRequests > 0 && s.models[modelIdx].ConcurrentCount >= s.models[modelIdx].MaxConcurrentRequests {
		s.mu.Unlock()
		return nil, &ProxyError{
			HTTPStatus: http.StatusTooManyRequests,
			Type:       "modelmux_rate_limited",
			Code:       "model_busy",
			Message:    fmt.Sprintf("model %s has reached its concurrency limit", model.ID),
		}
	}
	if modelIdx >= 0 {
		s.models[modelIdx].ConcurrentCount++
		s.models[modelIdx].MinuteRequestCount++
	}
	s.mu.Unlock()

	modelReleased := false
	if modelIdx >= 0 && !isStreamRequest(bodyBytes) {
		defer func() {
			s.mu.Lock()
			s.models[modelIdx].ConcurrentCount--
			s.mu.Unlock()
		}()
	} else if modelIdx >= 0 {
		defer func() {
			if !modelReleased {
				s.mu.Lock()
				s.models[modelIdx].ConcurrentCount--
				s.mu.Unlock()
			}
		}()
	}

	retried := map[string]int{}
	maxRetryPerKey := s.cfg.Retry.MaxRetryPerKey
	maxTotal := s.cfg.Retry.MaxTotalAttempts
	totalKeys := s.keyCountForModel(model.ID)
	if requiredKeyID != "" {
		totalKeys = 1
	}
	if maxTotal <= 0 {
		maxTotal = totalKeys * (maxRetryPerKey + 1)
	}
	if maxTotal == 0 {
		return nil, AllKeysUnavailableError(model.ID)
	}

	skipCount := 0
	for totalAttempts := 0; totalAttempts < maxTotal; {
		key, err := s.selectRequestKey(ctx, model.ID, requiredKeyID)
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

		s.mu.Lock()
		slotAcquired := false
		for i := range s.keys {
			if s.keys[i].ID == key.ID {
				if s.acquireKeySlotLocked(i) {
					slotAcquired = true
				}
				break
			}
		}
		s.mu.Unlock()
		if !slotAcquired {
			continue
		}

		clonedReq := cloneRequestWithBody(req, bodyBytes)
		startedAt := time.Now()
		client := s.clientReg.Get(provider.Type)
		resp, err := client.Forward(ctx, provider, model, *key, clonedReq, apiPath)
		if err != nil && key.Value != "" {
			err = fmt.Errorf("provider error: %s", redactSecret(err.Error(), key.Value))
		}
		result := classifyResult(resp, err, s.cfg)
		result.ModelID = model.ID
		result.GroupID = groupID
		result.ProviderID = provider.ID
		result.LatencyMs = time.Since(startedAt).Milliseconds()

		if result.Success && !isStreamRequest(bodyBytes) && resp != nil {
			respBodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil {
				result.TokenInput, result.TokenOutput = parseTokenUsage(respBodyBytes)
				resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
			}
		}

		if isStreamRequest(bodyBytes) && result.Success {
			modelReleased = true
			ctx = SetStreamResultContext(ctx, streamResultInfo{
				KeyID:      key.ID,
				ModelID:    model.ID,
				ProviderID: provider.ID,
				GroupID:    groupID,
				StatusCode: result.StatusCode,
				Error:      result.Error,
				StartedAt:  startedAt,
				ModelIdx:   modelIdx,
			})
			if resp != nil && resp.Body != nil {
				if tracker, ok := resp.Body.(providerclient.StreamUsageTracker); ok {
					ctx = context.WithValue(ctx, ctxKeyTokenTracker, tracker.StreamUsage())
				}
			}
			*req = *req.WithContext(ctx)
			return resp, nil
		}

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

	for i := range s.keys {
		if s.keys[i].ID != keyID {
			continue
		}
		s.releaseKeySlotLocked(i)
		now := time.Now()
		s.keys[i].UpdatedAt = now
		s.keys[i].UsedCount++
		s.keys[i].LastUsedAt = &now
		s.checkDailyResetLocked(i)
		s.recordKeyPerMinuteUsageLocked(i, result.TokenInput+result.TokenOutput)
		log := domain.RequestLog{ID: fmt.Sprintf("log-%d", now.UnixNano()), RequestID: GetRequestID(ctx), GroupID: result.GroupID, ModelID: result.ModelID, ProviderID: result.ProviderID, KeyID: keyID, StatusCode: result.StatusCode, Error: result.Error, LatencyMs: result.LatencyMs, TokenInput: result.TokenInput, TokenOutput: result.TokenOutput, CreatedAt: now}
		if result.Success {
			s.keys[i].ErrorCount = 0
			if s.keys[i].Status != domain.KeyStatusLimited {
				s.keys[i].Status = domain.KeyStatusActive
			}
			s.keys[i].CooldownEnd = nil
			s.applyDailyLimitsLocked(i, result.TokenInput, result.TokenOutput)
			logRecord := s.appendLogLocked(log)
			keyRecord := s.keyRuntimeRecord(s.keys[i])
			s.mu.Unlock()
			s.persistRequestLog(logRecord)
			s.persistKeyRuntime(keyRecord)
			return nil
		}
		s.keys[i].ErrorCount++
		if s.keys[i].Status != domain.KeyStatusLimited {
			s.keys[i].Status = domain.KeyStatusActive
		}
		if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
			s.keys[i].Status = domain.KeyStatusInvalid
			s.keys[i].CooldownEnd = nil
			logRecord := s.appendLogLocked(log)
			keyRecord := s.keyRuntimeRecord(s.keys[i])
			s.mu.Unlock()
			s.persistRequestLog(logRecord)
			s.persistKeyRuntime(keyRecord)
			return nil
		}
		if result.CooldownSeconds > 0 {
			s.keys[i].Status = domain.KeyStatusCooldown
			until := now.Add(time.Duration(result.CooldownSeconds) * time.Second)
			s.keys[i].CooldownEnd = &until
		}
		logRecord := s.appendLogLocked(log)
		keyRecord := s.keyRuntimeRecord(s.keys[i])
		s.mu.Unlock()
		s.persistRequestLog(logRecord)
		s.persistKeyRuntime(keyRecord)
		return nil
	}
	s.mu.Unlock()
	return fmt.Errorf("key %s not found", keyID)
}

func (s *RouterService) keyRuntimeRecord(k domain.APIKey) *storage.KeyRuntimeRecord {
	if s.store == nil {
		return nil
	}
	cooldownEnd := ""
	if k.CooldownEnd != nil {
		cooldownEnd = k.CooldownEnd.Format(time.RFC3339)
	}
	lastUsedAt := ""
	if k.LastUsedAt != nil {
		lastUsedAt = k.LastUsedAt.Format(time.RFC3339)
	}
	updatedAt := k.UpdatedAt.Format(time.RFC3339)
	return &storage.KeyRuntimeRecord{
		KeyID:             k.ID,
		Status:            string(k.Status),
		UsedCount:         k.UsedCount,
		ErrorCount:        k.ErrorCount,
		CooldownEnd:       cooldownEnd,
		LastUsedAt:        lastUsedAt,
		UpdatedAt:         updatedAt,
		DailyRequestCount: k.DailyRequestCount,
		DailyTokenCount:   k.DailyTokenCount,
		DailyDate:         k.DailyDate,
		DailyRequestLimit: k.DailyRequestLimit,
		DailyTokenLimit:   k.DailyTokenLimit,
	}
}

func (s *RouterService) persistKeyRuntime(record *storage.KeyRuntimeRecord) {
	if record == nil || s.store == nil {
		return
	}
	if err := s.store.SaveKeyRuntime(*record); err != nil {
		fmt.Fprintf(os.Stderr, "modelmux: storage write error: %v\n", err)
	}
}

func (s *RouterService) checkDailyResetLocked(idx int) {
	k := &s.keys[idx]
	today := time.Now().Format("2006-01-02")
	if k.DailyDate != today {
		k.DailyRequestCount = 0
		k.DailyTokenCount = 0
		k.DailyDate = today
		if k.Status == domain.KeyStatusLimited {
			k.Status = domain.KeyStatusActive
		}
	}
}

func (s *RouterService) applyDailyLimitsLocked(idx int, tokenInput, tokenOutput int) {
	k := &s.keys[idx]
	if k.Status == domain.KeyStatusLimited {
		return
	}
	k.DailyRequestCount++
	k.DailyTokenCount += tokenInput + tokenOutput
	limited := false
	if k.DailyRequestLimit > 0 && k.DailyRequestCount >= k.DailyRequestLimit {
		limited = true
	}
	if k.DailyTokenLimit > 0 && k.DailyTokenCount >= k.DailyTokenLimit {
		limited = true
	}
	if limited {
		k.Status = domain.KeyStatusLimited
	}
}

func (s *RouterService) FinalizeStreamResult(ctx context.Context, copyErr error) {
	info, ok := getStreamResultContext(ctx)
	if !ok {
		return
	}

	result := inbound.KeyResult{
		Success:    copyErr == nil,
		ModelID:    info.ModelID,
		GroupID:    info.GroupID,
		ProviderID: info.ProviderID,
		StatusCode: info.StatusCode,
	}
	if copyErr != nil {
		result.Error = "stream copy error: " + copyErr.Error()
		result.ShouldRotateKey = true
	}
	if !info.StartedAt.IsZero() {
		result.LatencyMs = time.Since(info.StartedAt).Milliseconds()
	}
	if tracker, ok := ctx.Value(ctxKeyTokenTracker).(*providerclient.StreamUsage); ok {
		result.TokenInput, result.TokenOutput = tracker.Tokens()
	}

	s.mu.Lock()
	if info.ModelIdx >= 0 {
		s.releaseModelSlotLocked(info.ModelIdx)
	}
	s.mu.Unlock()

	_ = s.MarkKeyResult(ctx, info.KeyID, result)
}

func (s *RouterService) saveTraceIfEnabled(traceID string) {
	if s.store == nil {
		return
	}
	trace, ok := s.aiTracer.GetTrace(traceID)
	if !ok {
		return
	}
	stepsBytes, _ := json.Marshal(trace.Steps)
	_ = s.store.SaveRouteTrace(storage.RouteTraceRecord{
		ID:            trace.ID,
		RequestID:     trace.RequestID,
		OriginalModel: trace.OriginalModel,
		ReroutedModel: trace.ReroutedModel,
		StepsJSON:     string(stepsBytes),
		CreatedAt:     trace.CreatedAt.Format(time.RFC3339),
	})
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

	client := s.clientReg.Get(provider.Type)
	return client.TestKey(ctx, provider, model, key)
}

func (s *RouterService) SetKeyStatus(keyID string, status string) {
	s.mu.Lock()
	for i := range s.keys {
		if s.keys[i].ID == keyID {
			s.keys[i].Status = domain.APIKeyStatus(status)
			if status == "active" {
				s.keys[i].CooldownEnd = nil
			}
			keyRecord := s.keyRuntimeRecord(s.keys[i])
			s.mu.Unlock()
			s.persistKeyRuntime(keyRecord)
			return
		}
	}
	s.mu.Unlock()
}

func (s *RouterService) appendLogLocked(log domain.RequestLog) *storage.RequestLogRecord {
	s.logs = append(s.logs, log)
	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
	if s.store == nil {
		return nil
	}
	createdAt := ""
	if !log.CreatedAt.IsZero() {
		createdAt = log.CreatedAt.Format(time.RFC3339)
	}
	return &storage.RequestLogRecord{
		ID:          log.ID,
		GroupID:     log.GroupID,
		ModelID:     log.ModelID,
		ProviderID:  log.ProviderID,
		KeyID:       log.KeyID,
		StatusCode:  log.StatusCode,
		Error:       log.Error,
		LatencyMs:   log.LatencyMs,
		TokenInput:  log.TokenInput,
		TokenOutput: log.TokenOutput,
		CreatedAt:   createdAt,
	}
}

func (s *RouterService) persistRequestLog(record *storage.RequestLogRecord) {
	if record == nil || s.store == nil {
		return
	}
	if err := s.store.SaveRequestLog(*record); err != nil {
		fmt.Fprintf(os.Stderr, "modelmux: storage write error: %v\n", err)
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

func (s *RouterService) keyByID(id string) (domain.APIKey, bool) {
	for _, k := range s.keys {
		if k.ID == id {
			return k, true
		}
	}
	return domain.APIKey{}, false
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
		if _, seen := attempted[groupMemberAttemptKey(member)]; seen {
			continue
		}
		modelID := member.ModelID
		if member.KeyID != "" {
			key, ok := s.keyByID(member.KeyID)
			if !ok {
				continue
			}
			modelID = key.ModelID
		}
		model, ok := s.modelByID(modelID)
		if !ok || !model.Enabled {
			continue
		}
		provider, ok := s.providerByID(model.ProviderID)
		if !ok || !provider.Enabled {
			continue
		}
		if member.KeyID != "" {
			if !s.isKeyAvailableForModel(member.KeyID, model.ID) {
				continue
			}
		} else if len(s.availableKeys(member.ModelID)) == 0 {
			continue
		}
		members = append(members, availableGroupMember{member: member, model: model})
	}
	return members
}

func groupMemberAttemptKey(member domain.ModelGroupMember) string {
	if member.KeyID != "" {
		return "key:" + member.KeyID
	}
	return "model:" + member.ModelID
}

func (s *RouterService) isKeyAvailableForModel(keyID string, modelID string) bool {
	for _, key := range s.availableKeys(modelID) {
		if key.ID == keyID {
			return true
		}
	}
	return false
}

func (s *RouterService) availableKeys(modelID string) []domain.APIKey {
	todayStr := time.Now().Format("2006-01-02")
	now := time.Now()
	var out []domain.APIKey
	for i, k := range s.keys {
		if k.ModelID != modelID {
			continue
		}
		if k.Status == domain.KeyStatusDisabled || k.Status == domain.KeyStatusInvalid {
			continue
		}
		if k.Status == domain.KeyStatusLimited {
			if k.DailyDate != todayStr {
				s.keys[i].Status = domain.KeyStatusActive
				s.keys[i].DailyRequestCount = 0
				s.keys[i].DailyTokenCount = 0
				s.keys[i].DailyDate = todayStr
				k = s.keys[i]
			} else {
				continue
			}
		}
		if k.Status == domain.KeyStatusCooldown && k.CooldownEnd != nil && k.CooldownEnd.After(now) {
			continue
		}
		if s.isKeyPerMinuteLimited(i) || s.isKeyConcurrencyLimited(i) {
			continue
		}
		out = append(out, k)
	}
	return out
}

func (s *RouterService) isKeyPerMinuteLimited(i int) bool {
	k := &s.keys[i]
	if k.RequestsPerMinute <= 0 && k.TokensPerMinute <= 0 {
		return false
	}
	now := time.Now()
	windowStart := k.MinuteWindowStart
	if windowStart.IsZero() || now.Sub(windowStart) >= time.Minute {
		k.MinuteWindowStart = now.Truncate(time.Minute)
		k.MinuteRequestCount = 0
		k.MinuteTokenCount = 0
		return false
	}
	if k.RequestsPerMinute > 0 && k.MinuteRequestCount >= k.RequestsPerMinute {
		return true
	}
	if k.TokensPerMinute > 0 && k.MinuteTokenCount >= k.TokensPerMinute {
		return true
	}
	return false
}

func (s *RouterService) isKeyConcurrencyLimited(i int) bool {
	k := &s.keys[i]
	if k.MaxConcurrentRequests <= 0 {
		return false
	}
	return k.ConcurrentCount >= k.MaxConcurrentRequests
}

func (s *RouterService) acquireKeySlotLocked(i int) bool {
	k := &s.keys[i]
	if k.MaxConcurrentRequests > 0 && k.ConcurrentCount >= k.MaxConcurrentRequests {
		return false
	}
	k.ConcurrentCount++
	return true
}

func (s *RouterService) releaseKeySlotLocked(i int) {
	k := &s.keys[i]
	if k.ConcurrentCount > 0 {
		k.ConcurrentCount--
	}
}

func (s *RouterService) recordKeyPerMinuteUsageLocked(i int, tokens int) {
	k := &s.keys[i]
	if k.RequestsPerMinute <= 0 && k.TokensPerMinute <= 0 {
		return
	}
	now := time.Now()
	if k.MinuteWindowStart.IsZero() || now.Sub(k.MinuteWindowStart) >= time.Minute {
		k.MinuteWindowStart = now.Truncate(time.Minute)
		k.MinuteRequestCount = 0
		k.MinuteTokenCount = 0
	}
	k.MinuteRequestCount++
	if tokens > 0 {
		k.MinuteTokenCount += tokens
	}
}

func (s *RouterService) clearKeyCooldown(keyID string) {
	s.mu.Lock()

	for i := range s.keys {
		if s.keys[i].ID == keyID {
			s.keys[i].Status = domain.KeyStatusActive
			s.keys[i].CooldownEnd = nil
			keyRecord := s.keyRuntimeRecord(s.keys[i])
			s.mu.Unlock()
			s.persistKeyRuntime(keyRecord)
			return
		}
	}
	s.mu.Unlock()
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

func isStreamRequest(body []byte) bool {
	var payload struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Stream
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

func redactSecret(text, secret string) string {
	if secret == "" || len(secret) < 4 {
		return text
	}
	return strings.ReplaceAll(text, secret, "***REDACTED***")
}

func parseTokenUsage(body []byte) (int, int) {
	var payload struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	return payload.Usage.PromptTokens, payload.Usage.CompletionTokens
}

func isUnavailable(err error) bool {
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		return false
	}
	return proxyErr.Code == "all_keys_limited" || proxyErr.Code == "all_group_models_unavailable"
}

func (s *RouterService) modelIndexByID(id string) int {
	for i, m := range s.models {
		if m.ID == id {
			return i
		}
	}
	return -1
}

func (s *RouterService) acquireModelSlotLocked(i int) bool {
	m := &s.models[i]
	if m.MaxConcurrentRequests > 0 && m.ConcurrentCount >= m.MaxConcurrentRequests {
		return false
	}
	m.ConcurrentCount++
	return true
}

func (s *RouterService) releaseModelSlotLocked(i int) {
	m := &s.models[i]
	if m.ConcurrentCount > 0 {
		m.ConcurrentCount--
	}
}

func defaultCapabilitiesForType(providerType domain.ProviderType, override *config.ModelCapabilityConfig) domain.Capabilities {
	caps := domain.Capabilities{Chat: true, Completions: true, Streaming: true, Tools: false, Vision: false, JSONMode: false}
	switch providerType {
	case domain.ProviderTypeAnthropic, domain.ProviderTypeGemini:
		caps.Completions = false
	case domain.ProviderTypeOpenAICompatible, domain.ProviderTypeCustom:
		caps.Tools = true
		caps.JSONMode = true
	}
	if override == nil {
		return caps
	}
	if override.Chat != nil {
		caps.Chat = *override.Chat
	}
	if override.Completions != nil {
		caps.Completions = *override.Completions
	}
	if override.Streaming != nil {
		caps.Streaming = *override.Streaming
	}
	if override.Tools != nil {
		caps.Tools = *override.Tools
	}
	if override.Vision != nil {
		caps.Vision = *override.Vision
	}
	if override.JSONMode != nil {
		caps.JSONMode = *override.JSONMode
	}
	return caps
}

func (s *RouterService) checkModelCapability(model domain.Model, apiPath string, bodyBytes []byte) error {
	if apiPath == "/completions" && !model.Capabilities.Completions {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_completions",
			Message:    fmt.Sprintf("model %s does not support /v1/completions", model.ID),
		}
	}
	if apiPath == "/chat/completions" && !model.Capabilities.Chat {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_chat",
			Message:    fmt.Sprintf("model %s does not support /v1/chat/completions", model.ID),
		}
	}
	if !model.Capabilities.Streaming && isStreamRequest(bodyBytes) {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_streaming",
			Message:    fmt.Sprintf("model %s does not support streaming", model.ID),
		}
	}
	if !model.Capabilities.Tools && hasToolsField(bodyBytes) {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_tools",
			Message:    fmt.Sprintf("model %s does not support tools", model.ID),
		}
	}
	if !model.Capabilities.Vision && hasImageContent(bodyBytes) {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_vision",
			Message:    fmt.Sprintf("model %s does not support vision/image input", model.ID),
		}
	}
	if !model.Capabilities.JSONMode && hasJSONMode(bodyBytes) {
		return &ProxyError{
			HTTPStatus: http.StatusBadRequest,
			Type:       "modelmux_unsupported",
			Code:       "capability_json_mode",
			Message:    fmt.Sprintf("model %s does not support json_mode", model.ID),
		}
	}
	return nil
}

func hasToolsField(body []byte) bool {
	var payload struct {
		Tools      []any `json:"tools"`
		ToolChoice any   `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return len(payload.Tools) > 0 || payload.ToolChoice != nil
}

func hasImageContent(body []byte) bool {
	var payload struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	for _, msg := range payload.Messages {
		s := string(msg.Content)
		if strings.Contains(s, `"image_url"`) || strings.Contains(s, `"type":"image"`) {
			return true
		}
	}
	return false
}

func hasJSONMode(body []byte) bool {
	var payload struct {
		ResponseFormat *struct {
			Type string `json:"type"`
		} `json:"response_format"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.ResponseFormat != nil && strings.ToLower(payload.ResponseFormat.Type) == "json_object"
}
