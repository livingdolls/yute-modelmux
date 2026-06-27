package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	type modelItem struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	models := s.rs.ListModels()
	groups := s.rs.ListModelGroups()
	items := make([]modelItem, 0, len(models)+len(groups))
	for _, m := range models {
		if !m.Enabled {
			continue
		}
		items = append(items, modelItem{ID: m.ID, Object: "model"})
	}
	for _, g := range groups {
		if !g.Enabled {
			continue
		}
		items = append(items, modelItem{ID: g.ID, Object: "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items})
}

func (s *Server) completionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	maxBytes := int64(s.cfg.Server.MaxRequestBodyMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	resp, err := s.rs.HandleCompletion(r.Context(), r)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if err := copyWithFlush(w, resp.Body); err != nil {
		s.rs.FinalizeStreamResult(r.Context(), err)
		return
	}
	s.rs.FinalizeStreamResult(r.Context(), nil)
}

func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	maxBytes := int64(s.cfg.Server.MaxRequestBodyMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	resp, err := s.rs.HandleChatCompletion(r.Context(), r)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if err := copyWithFlush(w, resp.Body); err != nil {
		s.rs.FinalizeStreamResult(r.Context(), err)
		return
	}
	s.rs.FinalizeStreamResult(r.Context(), nil)
}

func copyWithFlush(dst io.Writer, src io.Reader) error {
	flusher, canFlush := dst.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	type modelMetric struct {
		ID           string `json:"id"`
		Requests     int    `json:"requests"`
		Errors       int    `json:"errors"`
		ActiveKeys   int    `json:"active_keys"`
		CooldownKeys int    `json:"cooldown_keys"`
		InvalidKeys  int    `json:"invalid_keys"`
	}
	type groupMetric struct {
		ID                string `json:"id"`
		Requests          int    `json:"requests"`
		Errors            int    `json:"errors"`
		ActiveModels      int    `json:"active_models"`
		UnavailableModels int    `json:"unavailable_models"`
	}
	keys := s.rs.ListKeys()
	logs := s.rs.Logs()
	models := s.rs.ListModels()
	groups := s.rs.ListModelGroups()
	metrics := make([]modelMetric, 0, len(models))
	for _, model := range models {
		metric := modelMetric{ID: model.ID}
		for _, key := range keys {
			if key.ModelID != model.ID {
				continue
			}
			switch key.Status {
			case "active":
				metric.ActiveKeys++
			case "cooldown":
				metric.CooldownKeys++
			case "invalid":
				metric.InvalidKeys++
			}
		}
		for _, log := range logs {
			if log.ModelID != model.ID {
				continue
			}
			metric.Requests++
			if log.StatusCode >= 400 || log.Error != "" {
				metric.Errors++
			}
		}
		metrics = append(metrics, metric)
	}
	groupMetrics := make([]groupMetric, 0, len(groups))
	for _, group := range groups {
		metric := groupMetric{ID: group.ID}
		for _, member := range group.Members {
			active := false
			for _, model := range models {
				if model.ID == member.ModelID && model.Enabled && member.Enabled {
					active = modelHasActiveKey(keys, model.ID)
					break
				}
			}
			if active {
				metric.ActiveModels++
			} else {
				metric.UnavailableModels++
			}
		}
		for _, log := range logs {
			if log.GroupID != group.ID {
				continue
			}
			metric.Requests++
			if log.StatusCode >= 400 || log.Error != "" {
				metric.Errors++
			}
		}
		groupMetrics = append(groupMetrics, metric)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": metrics, "groups": groupMetrics})
}

func modelHasActiveKey(keys []domain.APIKey, modelID string) bool {
	now := time.Now()
	for _, key := range keys {
		if key.ModelID != modelID {
			continue
		}
		if key.Status == domain.KeyStatusDisabled || key.Status == domain.KeyStatusInvalid {
			continue
		}
		if key.Status == domain.KeyStatusCooldown && key.CooldownEnd != nil && key.CooldownEnd.After(now) {
			continue
		}
		return true
	}
	return false
}

func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset, _ := strconv.Atoi(query.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	modelFilter := query.Get("model_id")
	keyFilter := query.Get("key_id")
	providerFilter := query.Get("provider_id")
	groupFilter := query.Get("group_id")
	statusCodeFilter, _ := strconv.Atoi(query.Get("status_code"))

	logs := s.rs.Logs()
	sort.SliceStable(logs, func(i, j int) bool {
		return logs[i].CreatedAt.After(logs[j].CreatedAt)
	})

	var filtered []domain.RequestLog
	for _, log := range logs {
		if modelFilter != "" && log.ModelID != modelFilter {
			continue
		}
		if keyFilter != "" && log.KeyID != keyFilter {
			continue
		}
		if providerFilter != "" && log.ProviderID != providerFilter {
			continue
		}
		if groupFilter != "" && log.GroupID != groupFilter {
			continue
		}
		if statusCodeFilter > 0 && log.StatusCode != statusCodeFilter {
			continue
		}
		filtered = append(filtered, log)
	}

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	type logItem struct {
		ID          string `json:"id"`
		GroupID     string `json:"group_id"`
		ModelID     string `json:"model_id"`
		ProviderID  string `json:"provider_id"`
		KeyID       string `json:"key_id"`
		StatusCode  int    `json:"status_code"`
		Error       string `json:"error,omitempty"`
		LatencyMs   int64  `json:"latency_ms"`
		TokenInput  int    `json:"token_input"`
		TokenOutput int    `json:"token_output"`
		CreatedAt   string `json:"created_at"`
	}

	items := make([]logItem, len(page))
	for i, log := range page {
		createdAt := ""
		if !log.CreatedAt.IsZero() {
			createdAt = log.CreatedAt.Format(time.RFC3339)
		}
		items[i] = logItem{
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

	writeJSON(w, http.StatusOK, map[string]any{"logs": items, "total": total, "limit": limit, "offset": offset})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeProxyError(w http.ResponseWriter, err error) {
	var proxyErr *service.ProxyError
	if errors.As(err, &proxyErr) {
		writeJSON(w, proxyErr.HTTPStatus, map[string]any{"error": map[string]any{"message": proxyErr.Message, "type": proxyErr.Type, "code": proxyErr.Code}})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error(), "type": "modelmux_error"}})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Server.RequireAuth {
			next.ServeHTTP(w, r)
			return
		}
		expected := s.cfg.AuthToken()
		if expected == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server auth token is not configured"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+expected {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
