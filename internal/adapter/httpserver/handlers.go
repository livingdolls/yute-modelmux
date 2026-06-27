package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	storagepkg "github.com/livingdolls/yute-modelmux/internal/storage"
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

type metricSnapshot struct {
	requests     int
	errors       int
	rateLimits   int
	latencies    []int64
	activeKeys   int
	cooldownKeys int
	invalidKeys  int
	limitedKeys  int
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")

	keys := s.rs.ListKeys()
	logs := s.rs.LogsForMetrics()
	models := s.rs.ListModels()
	providers := s.rs.ListProviders()
	groups := s.rs.ListModelGroups()

	modelMetrics := map[string]*metricSnapshot{}
	for _, model := range models {
		modelMetrics[model.ID] = &metricSnapshot{}
	}
	for _, key := range keys {
		m, ok := modelMetrics[key.ModelID]
		if !ok {
			continue
		}
		switch key.Status {
		case domain.KeyStatusActive:
			m.activeKeys++
		case domain.KeyStatusCooldown:
			m.cooldownKeys++
		case domain.KeyStatusInvalid:
			m.invalidKeys++
		case domain.KeyStatusLimited:
			m.limitedKeys++
		}
	}
	for _, log := range logs {
		m, ok := modelMetrics[log.ModelID]
		if !ok {
			continue
		}
		m.requests++
		if log.StatusCode >= 400 || log.Error != "" {
			m.errors++
		}
		if log.StatusCode == http.StatusTooManyRequests {
			m.rateLimits++
		}
		if log.LatencyMs > 0 {
			m.latencies = append(m.latencies, log.LatencyMs)
		}
	}

	providerModelMap := map[string]string{}
	for _, model := range models {
		providerModelMap[model.ID] = model.ProviderID
	}

	if format == "prometheus" {
		s.writePrometheusMetrics(w, models, groups, keys, providers, logs, modelMetrics, providerModelMap)
		return
	}

	s.writeJSONMetrics(w, models, groups, keys, logs, modelMetrics)
}

func (s *Server) writeJSONMetrics(w http.ResponseWriter, models []domain.Model, groups []domain.ModelGroup, keys []domain.APIKey, logs []domain.RequestLog, modelMetrics map[string]*metricSnapshot) {
	type modelMetric struct {
		ID           string  `json:"id"`
		Requests     int     `json:"requests"`
		Errors       int     `json:"errors"`
		RateLimits   int     `json:"rate_limits"`
		LatencyAvgMs float64 `json:"latency_avg_ms"`
		LatencyP95Ms int64   `json:"latency_p95_ms"`
		ActiveKeys   int     `json:"active_keys"`
		CooldownKeys int     `json:"cooldown_keys"`
		InvalidKeys  int     `json:"invalid_keys"`
		LimitedKeys  int     `json:"limited_keys"`
	}
	type groupMetric struct {
		ID                string `json:"id"`
		Requests          int    `json:"requests"`
		Errors            int    `json:"errors"`
		ActiveModels      int    `json:"active_models"`
		UnavailableModels int    `json:"unavailable_models"`
	}

	mm := make([]modelMetric, 0, len(models))
	for _, model := range models {
		ms := modelMetrics[model.ID]
		if ms == nil {
			continue
		}
		latencyAvg := float64(0)
		latencyP95 := int64(0)
		if len(ms.latencies) > 0 {
			sorted := make([]int64, len(ms.latencies))
			copy(sorted, ms.latencies)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			var total int64
			for _, l := range sorted {
				total += l
			}
			latencyAvg = float64(total) / float64(len(sorted))
			p95Idx := int(float64(len(sorted)) * 0.95)
			if p95Idx >= len(sorted) {
				p95Idx = len(sorted) - 1
			}
			latencyP95 = sorted[p95Idx]
		}
		mm = append(mm, modelMetric{
			ID:           model.ID,
			Requests:     ms.requests,
			Errors:       ms.errors,
			RateLimits:   ms.rateLimits,
			LatencyAvgMs: latencyAvg,
			LatencyP95Ms: latencyP95,
			ActiveKeys:   ms.activeKeys,
			CooldownKeys: ms.cooldownKeys,
			InvalidKeys:  ms.invalidKeys,
			LimitedKeys:  ms.limitedKeys,
		})
	}

	gm := make([]groupMetric, 0, len(groups))
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
		gm = append(gm, metric)
	}

	writeJSON(w, http.StatusOK, map[string]any{"models": mm, "groups": gm})
}

func (s *Server) writePrometheusMetrics(w http.ResponseWriter, models []domain.Model, groups []domain.ModelGroup, keys []domain.APIKey, providers []domain.Provider, logs []domain.RequestLog, modelMetrics map[string]*metricSnapshot, providerModelMap map[string]string) {
	var b strings.Builder

	totalRequests := 0
	totalErrors := 0
	totalRateLimits := 0
	allLatencies := []int64{}
	for _, log := range logs {
		totalRequests++
		if log.StatusCode >= 400 || log.Error != "" {
			totalErrors++
		}
		if log.StatusCode == http.StatusTooManyRequests {
			totalRateLimits++
		}
		if log.LatencyMs > 0 {
			allLatencies = append(allLatencies, log.LatencyMs)
		}
	}

	activeKeys := 0
	cooldownKeys := 0
	invalidKeys := 0
	limitedKeys := 0
	for _, key := range keys {
		switch key.Status {
		case domain.KeyStatusActive:
			activeKeys++
		case domain.KeyStatusCooldown:
			cooldownKeys++
		case domain.KeyStatusInvalid:
			invalidKeys++
		case domain.KeyStatusLimited:
			limitedKeys++
		}
	}

	b.WriteString("# HELP modelmux_requests_total Total number of requests\n")
	b.WriteString("# TYPE modelmux_requests_total counter\n")
	b.WriteString(fmt.Sprintf("modelmux_requests_total %d\n", totalRequests))

	b.WriteString("# HELP modelmux_errors_total Total number of errors\n")
	b.WriteString("# TYPE modelmux_errors_total counter\n")
	b.WriteString(fmt.Sprintf("modelmux_errors_total %d\n", totalErrors))

	b.WriteString("# HELP modelmux_rate_limits_total Total number of rate limit responses\n")
	b.WriteString("# TYPE modelmux_rate_limits_total counter\n")
	b.WriteString(fmt.Sprintf("modelmux_rate_limits_total %d\n", totalRateLimits))

	if len(allLatencies) > 0 {
		sort.Slice(allLatencies, func(i, j int) bool { return allLatencies[i] < allLatencies[j] })
		var totalLatency int64
		for _, l := range allLatencies {
			totalLatency += l
		}
		avgMs := float64(totalLatency) / float64(len(allLatencies))
		p95Idx := int(float64(len(allLatencies)) * 0.95)
		if p95Idx >= len(allLatencies) {
			p95Idx = len(allLatencies) - 1
		}
		p95Ms := allLatencies[p95Idx]

		b.WriteString("# HELP modelmux_latency_avg_ms Average request latency in milliseconds\n")
		b.WriteString("# TYPE modelmux_latency_avg_ms gauge\n")
		b.WriteString(fmt.Sprintf("modelmux_latency_avg_ms %.0f\n", avgMs))

		b.WriteString("# HELP modelmux_latency_p95_ms P95 request latency in milliseconds\n")
		b.WriteString("# TYPE modelmux_latency_p95_ms gauge\n")
		b.WriteString(fmt.Sprintf("modelmux_latency_p95_ms %d\n", p95Ms))
	}

	b.WriteString("# HELP modelmux_active_keys Number of active API keys\n")
	b.WriteString("# TYPE modelmux_active_keys gauge\n")
	b.WriteString(fmt.Sprintf("modelmux_active_keys %d\n", activeKeys))

	b.WriteString("# HELP modelmux_cooldown_keys Number of keys in cooldown\n")
	b.WriteString("# TYPE modelmux_cooldown_keys gauge\n")
	b.WriteString(fmt.Sprintf("modelmux_cooldown_keys %d\n", cooldownKeys))

	b.WriteString("# HELP modelmux_invalid_keys Number of invalid API keys\n")
	b.WriteString("# TYPE modelmux_invalid_keys gauge\n")
	b.WriteString(fmt.Sprintf("modelmux_invalid_keys %d\n", invalidKeys))

	b.WriteString("# HELP modelmux_limited_keys Number of rate-limited API keys\n")
	b.WriteString("# TYPE modelmux_limited_keys gauge\n")
	b.WriteString(fmt.Sprintf("modelmux_limited_keys %d\n", limitedKeys))

	for _, model := range models {
		provider := providerModelMap[model.ID]
		ms := modelMetrics[model.ID]
		if ms == nil {
			continue
		}
		labels := fmt.Sprintf(`model="%s",provider="%s"`, model.ID, provider)
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, ms.requests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, ms.errors))
		b.WriteString(fmt.Sprintf("modelmux_rate_limits_total{%s} %d\n", labels, ms.rateLimits))

		if len(ms.latencies) > 0 {
			sorted := make([]int64, len(ms.latencies))
			copy(sorted, ms.latencies)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			var total int64
			for _, l := range sorted {
				total += l
			}
			avg := float64(total) / float64(len(sorted))
			p95Idx := int(float64(len(sorted)) * 0.95)
			if p95Idx >= len(sorted) {
				p95Idx = len(sorted) - 1
			}
			b.WriteString(fmt.Sprintf("modelmux_latency_avg_ms{%s} %.0f\n", labels, avg))
			b.WriteString(fmt.Sprintf("modelmux_latency_p95_ms{%s} %d\n", labels, sorted[p95Idx]))
		}
	}
	for _, key := range keys {
		keyRequests := 0
		keyErrors := 0
		for _, log := range logs {
			if log.KeyID != key.ID {
				continue
			}
			keyRequests++
			if log.StatusCode >= 400 || log.Error != "" {
				keyErrors++
			}
		}
		labels := fmt.Sprintf(`model="%s",provider="%s",key="%s"`, key.ModelID, key.ProviderID, key.ID)
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, keyRequests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, keyErrors))
	}

	for _, group := range groups {
		groupRequests := 0
		groupErrors := 0
		for _, log := range logs {
			if log.GroupID != group.ID {
				continue
			}
			groupRequests++
			if log.StatusCode >= 400 || log.Error != "" {
				groupErrors++
			}
		}
		labels := fmt.Sprintf(`group="%s"`, group.ID)
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, groupRequests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, groupErrors))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
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

	filter := storagepkg.LogFilter{
		ModelID:    query.Get("model_id"),
		KeyID:      query.Get("key_id"),
		ProviderID: query.Get("provider_id"),
		GroupID:    query.Get("group_id"),
		Limit:      limit,
		Offset:     offset,
	}
	statusCodeFilter, _ := strconv.Atoi(query.Get("status_code"))
	if statusCodeFilter > 0 {
		filter.StatusCode = statusCodeFilter
	}

	page, total := s.rs.QueryLogs(filter)

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
