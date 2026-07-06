package ai

import (
	"testing"
)

func TestRouteTracerLifecycle(t *testing.T) {
	tr := NewRouteTracer()
	trace := tr.StartTrace("req-1", "gpt-4")

	if trace.ID == "" {
		t.Fatal("expected trace ID")
	}
	if trace.RequestID != "req-1" {
		t.Fatalf("expected request ID req-1, got %s", trace.RequestID)
	}
	if trace.OriginalModel != "gpt-4" {
		t.Fatalf("expected original model gpt-4, got %s", trace.OriginalModel)
	}

	tr.AddStep(trace.ID, "classifier", "coding", "matched code keywords", "python")
	tr.AddStep(trace.ID, "guardrails", "allow", "passed all checks", "max_chars: ok")
	tr.FinalizeTrace(trace.ID, "deepseek-coder")

	retrieved, ok := tr.GetTrace(trace.ID)
	if !ok {
		t.Fatal("expected to retrieve trace by ID")
	}
	if len(retrieved.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(retrieved.Steps))
	}
	if retrieved.ReroutedModel != "deepseek-coder" {
		t.Fatalf("expected rerouted deepseek-coder, got %s", retrieved.ReroutedModel)
	}

	retrieved2, ok := tr.GetTraceByRequestID("req-1")
	if !ok {
		t.Fatal("expected to retrieve trace by request ID")
	}
	if retrieved2.ID != trace.ID {
		t.Fatal("expected same trace by request ID lookup")
	}
}

func TestRouteTracerMissingTrace(t *testing.T) {
	tr := NewRouteTracer()
	_, ok := tr.GetTrace("nonexistent")
	if ok {
		t.Fatal("expected false for missing trace")
	}
}
