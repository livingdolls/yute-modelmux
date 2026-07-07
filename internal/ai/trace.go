package ai

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type RouteTracer struct {
	mu     sync.Mutex
	traces map[string]*domain.RouteTrace
}

func NewRouteTracer() *RouteTracer {
	return &RouteTracer{traces: make(map[string]*domain.RouteTrace)}
}

func (t *RouteTracer) StartTrace(requestID, originalModel string) *domain.RouteTrace {
	id := traceID()
	trace := &domain.RouteTrace{
		ID:            id,
		RequestID:     requestID,
		OriginalModel: originalModel,
		CreatedAt:     time.Now(),
	}
	t.mu.Lock()
	t.traces[id] = trace
	t.mu.Unlock()
	return trace
}

func (t *RouteTracer) AddStep(traceID, stage, decision, reason, detail string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.traces[traceID]
	if !ok {
		return
	}
	trace.Steps = append(trace.Steps, domain.Step{
		Stage:    stage,
		Decision: decision,
		Reason:   reason,
		Detail:   detail,
	})
}

func (t *RouteTracer) GetTrace(traceID string) (*domain.RouteTrace, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.traces[traceID]
	return trace, ok
}

func (t *RouteTracer) GetTraceByRequestID(requestID string) (*domain.RouteTrace, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, trace := range t.traces {
		if trace.RequestID == requestID {
			return trace, true
		}
	}
	return nil, false
}

func (t *RouteTracer) FinalizeTrace(traceID, reroutedModel string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.traces[traceID]
	if !ok {
		return
	}
	trace.ReroutedModel = reroutedModel
}

func traceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (t *RouteTracer) ListTraces() []domain.RouteTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	traces := make([]domain.RouteTrace, 0, len(t.traces))
	for _, trace := range t.traces {
		traces = append(traces, *trace)
	}
	return traces
}
