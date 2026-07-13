package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	rs := s.routerServiceForRequest(r)
	models := rs.ListModels()
	groups := rs.ListModelGroups()
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
	maxBytes := int64(s.loadConfig().Server.MaxRequestBodyMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	rs := s.routerServiceForRequest(r)
	resp, err := rs.HandleCompletion(r.Context(), r)
	if err != nil {
		if traceID := service.GetTraceID(r.Context()); traceID != "" {
			w.Header().Set("X-ModelMux-Route-Trace-ID", traceID)
		}
		writeProxyError(w, err)
		return
	}
	defer resp.Body.Close()
	copyProxyHeaders(w.Header(), resp.Header)
	if traceID := service.GetTraceID(r.Context()); traceID != "" {
		w.Header().Set("X-ModelMux-Route-Trace-ID", traceID)
	}
	w.WriteHeader(resp.StatusCode)
	if err := copyWithFlush(w, resp.Body); err != nil {
		rs.FinalizeStreamResult(r.Context(), err)
		return
	}
	rs.FinalizeStreamResult(r.Context(), nil)
}

func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	maxBytes := int64(s.loadConfig().Server.MaxRequestBodyMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	rs := s.routerServiceForRequest(r)
	resp, err := rs.HandleChatCompletion(r.Context(), r)
	if err != nil {
		if traceID := service.GetTraceID(r.Context()); traceID != "" {
			w.Header().Set("X-ModelMux-Route-Trace-ID", traceID)
		}
		writeProxyError(w, err)
		return
	}
	defer resp.Body.Close()
	copyProxyHeaders(w.Header(), resp.Header)
	if traceID := service.GetTraceID(r.Context()); traceID != "" {
		w.Header().Set("X-ModelMux-Route-Trace-ID", traceID)
	}
	w.WriteHeader(resp.StatusCode)
	if err := copyWithFlush(w, resp.Body); err != nil {
		rs.FinalizeStreamResult(r.Context(), err)
		return
	}
	rs.FinalizeStreamResult(r.Context(), nil)
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

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
	"Content-Length",
}

func copyProxyHeaders(dst, src http.Header) {
	blocked := make(map[string]struct{}, len(hopByHopHeaders))
	for _, h := range hopByHopHeaders {
		blocked[http.CanonicalHeaderKey(h)] = struct{}{}
	}
	for _, connectionHeader := range src.Values("Connection") {
		for _, token := range strings.Split(connectionHeader, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				blocked[http.CanonicalHeaderKey(token)] = struct{}{}
			}
		}
	}
	for k, values := range src {
		if _, ok := blocked[http.CanonicalHeaderKey(k)]; ok {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

type metricSnapshot struct {
	requests       uint64
	errors         uint64
	rateLimits     uint64
	latencyCount   uint64
	latencySumMs   uint64
	latencyBuckets service.RuntimeLatencyBuckets
	statusClasses  map[string]uint64
	activeKeys     int
	cooldownKeys   int
	invalidKeys    int
	limitedKeys    int
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")

	rs := s.routerServiceForRequest(r)
	keys := rs.ListKeys()
	metrics := rs.MetricsSnapshot()
	models := rs.ListModels()
	groups := rs.ListModelGroups()

	modelMetrics := map[string]*metricSnapshot{}
	for _, model := range models {
		modelMetrics[model.ID] = metricSnapshotFromRuntime(metrics.Models[model.ID])
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
	groupMetrics := map[string]*metricSnapshot{}
	for _, group := range groups {
		groupMetrics[group.ID] = metricSnapshotFromRuntime(metrics.Groups[group.ID])
	}

	providerModelMap := map[string]string{}
	for _, model := range models {
		providerModelMap[model.ID] = model.ProviderID
	}

	if format == "prometheus" {
		s.writePrometheusMetrics(w, models, groups, keys, metrics, modelMetrics, groupMetrics, providerModelMap)
		return
	}

	s.writeJSONMetrics(w, models, groups, keys, modelMetrics, groupMetrics)
}

func metricSnapshotFromRuntime(src service.RuntimeMetricSeries) *metricSnapshot {
	return &metricSnapshot{
		requests:       src.Requests,
		errors:         src.Errors,
		rateLimits:     src.RateLimits,
		latencyCount:   src.LatencyCount,
		latencySumMs:   src.LatencySumMs,
		latencyBuckets: src.LatencyBuckets,
		statusClasses:  src.StatusClasses,
	}
}

func (s *Server) writeJSONMetrics(w http.ResponseWriter, models []domain.Model, groups []domain.ModelGroup, keys []domain.APIKey, modelMetrics map[string]*metricSnapshot, groupMetrics map[string]*metricSnapshot) {
	type modelMetric struct {
		ID           string  `json:"id"`
		Requests     uint64  `json:"requests"`
		Errors       uint64  `json:"errors"`
		RateLimits   uint64  `json:"rate_limits"`
		LatencyAvgMs float64 `json:"latency_avg_ms"`
		LatencyP95Ms int64   `json:"latency_p95_ms"`
		ActiveKeys   int     `json:"active_keys"`
		CooldownKeys int     `json:"cooldown_keys"`
		InvalidKeys  int     `json:"invalid_keys"`
		LimitedKeys  int     `json:"limited_keys"`
	}
	type groupMetric struct {
		ID                string `json:"id"`
		Requests          uint64 `json:"requests"`
		Errors            uint64 `json:"errors"`
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
		if ms.latencyCount > 0 {
			latencyAvg = float64(ms.latencySumMs) / float64(ms.latencyCount)
			latencyP95 = approximateP95FromBuckets(ms.latencyBuckets, ms.latencyCount)
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
		if gs := groupMetrics[group.ID]; gs != nil {
			metric.Requests = gs.requests
			metric.Errors = gs.errors
		}
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
		gm = append(gm, metric)
	}

	writeJSON(w, http.StatusOK, map[string]any{"models": mm, "groups": gm})
}

func (s *Server) writePrometheusMetrics(w http.ResponseWriter, models []domain.Model, groups []domain.ModelGroup, keys []domain.APIKey, metrics service.RuntimeMetrics, modelMetrics map[string]*metricSnapshot, groupMetrics map[string]*metricSnapshot, providerModelMap map[string]string) {
	var b strings.Builder

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
	b.WriteString(fmt.Sprintf("modelmux_requests_total %d\n", metrics.Requests))

	b.WriteString("# HELP modelmux_errors_total Total number of errors\n")
	b.WriteString("# TYPE modelmux_errors_total counter\n")
	b.WriteString(fmt.Sprintf("modelmux_errors_total %d\n", metrics.Errors))

	b.WriteString("# HELP modelmux_rate_limits_total Total number of rate limit responses\n")
	b.WriteString("# TYPE modelmux_rate_limits_total counter\n")
	b.WriteString(fmt.Sprintf("modelmux_rate_limits_total %d\n", metrics.RateLimits))

	if metrics.LatencyCount > 0 {
		avgMs := float64(metrics.LatencySumMs) / float64(metrics.LatencyCount)
		p95Ms := approximateP95FromBuckets(metrics.LatencyBuckets, metrics.LatencyCount)

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

	b.WriteString("# HELP modelmux_latency_ms Request latency in milliseconds\n")
	b.WriteString("# TYPE modelmux_latency_ms histogram\n")
	for _, model := range models {
		provider := providerModelMap[model.ID]
		ms := modelMetrics[model.ID]
		if ms == nil {
			continue
		}
		labels := fmt.Sprintf(`model="%s",provider="%s"`, escapeLabelValue(model.ID), escapeLabelValue(provider))
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, ms.requests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, ms.errors))
		b.WriteString(fmt.Sprintf("modelmux_rate_limits_total{%s} %d\n", labels, ms.rateLimits))

		if ms.latencyCount > 0 {
			avg := float64(ms.latencySumMs) / float64(ms.latencyCount)
			p95 := approximateP95FromBuckets(ms.latencyBuckets, ms.latencyCount)
			b.WriteString(fmt.Sprintf("modelmux_latency_avg_ms{%s} %.0f\n", labels, avg))
			b.WriteString(fmt.Sprintf("modelmux_latency_p95_ms{%s} %d\n", labels, p95))

			for i, upperBound := range service.RuntimeLatencyBucketsMs {
				b.WriteString(fmt.Sprintf("modelmux_latency_ms_bucket{%s,le=\"%d\"} %d\n", labels, upperBound, ms.latencyBuckets[i]))
			}
			b.WriteString(fmt.Sprintf("modelmux_latency_ms_bucket{%s,le=\"+Inf\"} %d\n", labels, ms.latencyBuckets[service.RuntimeLatencyInfBucket]))
			b.WriteString(fmt.Sprintf("modelmux_latency_ms_sum{%s} %d\n", labels, ms.latencySumMs))
			b.WriteString(fmt.Sprintf("modelmux_latency_ms_count{%s} %d\n", labels, ms.latencyCount))
		}

		for class, count := range ms.statusClasses {
			if count > 0 {
				b.WriteString(fmt.Sprintf("modelmux_status_total{%s,status_class=\"%s\"} %d\n", labels, class, count))
			}
		}
	}
	for _, key := range keys {
		keyMetric := metrics.Keys[key.ID]
		labels := fmt.Sprintf(`model="%s",provider="%s",key="%s"`, escapeLabelValue(key.ModelID), escapeLabelValue(key.ProviderID), escapeLabelValue(key.ID))
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, keyMetric.Requests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, keyMetric.Errors))
	}

	for _, group := range groups {
		groupMetric := groupMetrics[group.ID]
		labels := fmt.Sprintf(`group="%s"`, escapeLabelValue(group.ID))
		var requests uint64
		var errors uint64
		if groupMetric != nil {
			requests = groupMetric.requests
			errors = groupMetric.errors
		}
		b.WriteString(fmt.Sprintf("modelmux_requests_total{%s} %d\n", labels, requests))
		b.WriteString(fmt.Sprintf("modelmux_errors_total{%s} %d\n", labels, errors))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func approximateP95FromBuckets(buckets service.RuntimeLatencyBuckets, count uint64) int64 {
	if count == 0 {
		return 0
	}
	threshold := (count*95 + 99) / 100
	for i, cumulative := range buckets {
		if cumulative >= threshold {
			if i < len(service.RuntimeLatencyBucketsMs) {
				return service.RuntimeLatencyBucketsMs[i]
			}
			return service.RuntimeLatencyBucketsMs[len(service.RuntimeLatencyBucketsMs)-1]
		}
	}
	return 0
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

	page, total := s.routerServiceForRequest(r).QueryLogs(filter)

	type logItem struct {
		ID            string  `json:"id"`
		GroupID       string  `json:"group_id"`
		ModelID       string  `json:"model_id"`
		ProviderID    string  `json:"provider_id"`
		KeyID         string  `json:"key_id"`
		StatusCode    int     `json:"status_code"`
		Error         string  `json:"error,omitempty"`
		LatencyMs     int64   `json:"latency_ms"`
		TokenInput    int     `json:"token_input"`
		TokenOutput   int     `json:"token_output"`
		EstimatedCost float64 `json:"estimated_cost"`
		CreatedAt     string  `json:"created_at"`
	}

	items := make([]logItem, len(page))
	for i, log := range page {
		createdAt := ""
		if !log.CreatedAt.IsZero() {
			createdAt = log.CreatedAt.Format(time.RFC3339)
		}
		items[i] = logItem{
			ID:            log.ID,
			GroupID:       log.GroupID,
			ModelID:       log.ModelID,
			ProviderID:    log.ProviderID,
			KeyID:         log.KeyID,
			StatusCode:    log.StatusCode,
			Error:         log.Error,
			LatencyMs:     log.LatencyMs,
			TokenInput:    log.TokenInput,
			TokenOutput:   log.TokenOutput,
			EstimatedCost: log.EstimatedCost,
			CreatedAt:     createdAt,
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
		cfg := s.loadConfig()
		isAdmin := strings.HasPrefix(r.URL.Path, "/admin/")
		if isAdmin {
			if cfg.AdminRequireAuth() {
				if !authorized(r, cfg.AuthToken()) {
					writeAuthError(w, cfg.AuthToken(), "admin auth token is not configured")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !isLocalAddr(r.RemoteAddr) {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin endpoints require auth or local bind"})
				return
			}
		}
		if !cfg.Server.RequireAuth {
			next.ServeHTTP(w, r)
			return
		}
		expected := cfg.AuthToken()
		if !authorized(r, expected) {
			writeAuthError(w, expected, "server auth token is not configured")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authorized(r *http.Request, expected string) bool {
	return expected != "" && r.Header.Get("Authorization") == "Bearer "+expected
}

func writeAuthError(w http.ResponseWriter, expected string, missingMessage string) {
	if expected == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": missingMessage})
		return
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
}

func escapeLabelValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
