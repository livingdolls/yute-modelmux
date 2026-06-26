package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	providerclient "github.com/livingdolls/yute-modelmux/internal/adapter/provider"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
)

var ErrNoAvailableKey = errors.New("no available key")

type RouterService struct {
	cfg       *config.Config
	client    *providerclient.OpenAICompatibleClient
	mu        sync.Mutex
	rrIndex   map[string]int
	logs      []domain.RequestLog
	providers []domain.Provider
	models    []domain.Model
	keys      []domain.APIKey
}

func NewRouterService(cfg *config.Config) *RouterService {
	rs := &RouterService{
		cfg:     cfg,
		client:  providerclient.New(),
		rrIndex: map[string]int{},
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
	now := time.Now()
	for _, k := range cfg.Keys {
		status := domain.APIKeyStatus(k.Status)
		if status == "" {
			status = domain.KeyStatusActive
		}
		rs.keys = append(rs.keys, domain.APIKey{ID: k.ID, ProviderID: k.ProviderID, ModelID: k.ModelID, Name: k.Name, Value: k.Value, ValueEnv: k.ValueEnv, Status: status, Priority: k.Priority, CreatedAt: now, UpdatedAt: now})
	}
	return rs
}

func (s *RouterService) ListProviders() []domain.Provider {
	return append([]domain.Provider(nil), s.providers...)
}
func (s *RouterService) ListModels() []domain.Model { return append([]domain.Model(nil), s.models...) }
func (s *RouterService) ListKeys() []domain.APIKey  { return append([]domain.APIKey(nil), s.keys...) }
func (s *RouterService) Logs() []domain.RequestLog {
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
		return nil, err
	}
	modelID := extractModelFromBody(bodyBytes)
	if modelID == "" {
		return nil, errors.New("request body missing model")
	}
	model, ok := s.modelByID(modelID)
	if !ok {
		return nil, fmt.Errorf("unknown model %s", modelID)
	}
	provider, ok := s.providerByID(model.ProviderID)
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", model.ProviderID)
	}

	attempted := map[string]struct{}{}
	maxAttempts := len(s.keys)
	if s.cfg.Retry.MaxTotalAttempts > 0 && s.cfg.Retry.MaxTotalAttempts < maxAttempts {
		maxAttempts = s.cfg.Retry.MaxTotalAttempts
	}

	for attempts := 0; attempts < maxAttempts; attempts++ {
		key, err := s.SelectKey(ctx, modelID)
		if err != nil {
			return nil, err
		}
		if _, seen := attempted[key.ID]; seen {
			return nil, errors.New("all keys attempted")
		}
		attempted[key.ID] = struct{}{}

		clonedReq := cloneRequestWithBody(req, bodyBytes)
		resp, err := s.client.ForwardChatCompletion(ctx, provider, model, *key, clonedReq)
		result := classifyResult(resp, err, s.cfg)
		_ = s.MarkKeyResult(ctx, key.ID, result)
		if result.Success {
			return resp, nil
		}
		if !result.ShouldRotateKey {
			return resp, err
		}
	}
	return nil, errors.New("all keys failed")
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
		if result.Success {
			s.keys[i].ErrorCount = 0
			s.keys[i].Status = domain.KeyStatusActive
			s.keys[i].LastUsedAt = &now
			return nil
		}
		s.keys[i].ErrorCount++
		s.keys[i].LastUsedAt = &now
		s.keys[i].Status = domain.KeyStatusActive
		s.appendLog(domain.RequestLog{ID: fmt.Sprintf("log-%d", now.UnixNano()), KeyID: keyID, StatusCode: result.StatusCode, Error: result.Error, CreatedAt: now})
		if result.StatusCode == http.StatusTooManyRequests {
			s.keys[i].Status = domain.KeyStatusCooldown
			until := now.Add(time.Duration(result.CooldownSeconds) * time.Second)
			s.keys[i].CooldownEnd = &until
		}
		if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
			s.keys[i].Status = domain.KeyStatusInvalid
		}
		return nil
	}
	return fmt.Errorf("key %s not found", keyID)
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

func cloneRequestWithBody(req *http.Request, body []byte) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.Header = req.Header.Clone()
	return cloned
}

func extractModelFromBody(body []byte) string {
	marker := `"model"`
	idx := strings.Index(string(body), marker)
	if idx < 0 {
		return ""
	}
	rest := string(body)[idx+len(marker):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = rest[colon+1:]
	rest = strings.TrimLeft(rest, " \t\r\n\"")
	end := strings.IndexAny(rest, "\" ,}\r\n\t")
	if end < 0 {
		return strings.Trim(rest, "\"")
	}
	return strings.Trim(rest[:end], "\"")
}

func classifyResult(resp *http.Response, err error, cfg *config.Config) inbound.KeyResult {
	if err != nil {
		return inbound.KeyResult{ShouldRotateKey: true, Error: err.Error()}
	}
	if resp == nil {
		return inbound.KeyResult{ShouldRotateKey: true, Error: "empty response"}
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
		}
	}
	if !result.Success && !result.ShouldRotateKey {
		result.Error = resp.Status
	}
	return result
}
