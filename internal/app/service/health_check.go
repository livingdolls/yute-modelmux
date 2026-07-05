package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

type HealthChecker struct {
	mu        sync.RWMutex
	rs        *RouterService
	cfg       config.HealthCheckConfig
	parentCtx context.Context
	cancel    context.CancelFunc
}

func NewHealthChecker(rs *RouterService, cfg config.HealthCheckConfig) *HealthChecker {
	return &HealthChecker{rs: rs, cfg: cfg}
}

func (h *HealthChecker) Start(parentCtx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parentCtx = parentCtx
	h.stopLocked()
	h.startLocked()
}

func (h *HealthChecker) Update(rs *RouterService, cfg config.HealthCheckConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rs = rs
	h.cfg = cfg
	if h.parentCtx == nil {
		return
	}
	h.stopLocked()
	h.startLocked()
}

func (h *HealthChecker) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked()
}

func (h *HealthChecker) Router() *RouterService {
	rs, _ := h.snapshot()
	return rs
}

func (h *HealthChecker) startLocked() {
	if !h.cfg.Enabled || h.cfg.IntervalSeconds <= 0 {
		return
	}
	interval := time.Duration(h.cfg.IntervalSeconds) * time.Second
	ctx, cancel := context.WithCancel(h.parentCtx)
	h.cancel = cancel

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.runCheck(ctx)
			}
		}
	}()
}

func (h *HealthChecker) stopLocked() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *HealthChecker) snapshot() (*RouterService, config.HealthCheckConfig) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rs, h.cfg
}

func (h *HealthChecker) runCheck(ctx context.Context) {
	rs, cfg := h.snapshot()
	if rs == nil {
		return
	}
	keys := rs.ListKeys()
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, key := range keys {
		if key.Status == "disabled" {
			continue
		}
		select {
		case <-checkCtx.Done():
			return
		default:
		}

		err := rs.TestKey(checkCtx, key.ID)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") {
				h.markInvalid(rs, key.ID)
			} else {
				h.markCooldown(rs, key.ID)
			}
		} else {
			h.markRecovered(rs, key.ID)
		}
	}
}

func (h *HealthChecker) markInvalid(rs *RouterService, keyID string) {
	var keyRecord *storage.KeyRuntimeRecord
	rs.mu.Lock()
	for i := range rs.keys {
		if rs.keys[i].ID == keyID {
			if rs.keys[i].Status != "disabled" && rs.keys[i].Status != "invalid" {
				rs.keys[i].Status = "invalid"
				rs.keys[i].CooldownEnd = nil
				keyRecord = rs.keyRuntimeRecord(rs.keys[i])
			}
			break
		}
	}
	rs.mu.Unlock()
	rs.persistKeyRuntime(keyRecord)
}

func (h *HealthChecker) markCooldown(rs *RouterService, keyID string) {
	var keyRecord *storage.KeyRuntimeRecord
	rs.mu.Lock()
	for i := range rs.keys {
		if rs.keys[i].ID == keyID {
			if rs.keys[i].Status == "disabled" || rs.keys[i].Status == "invalid" {
				break
			}
			rs.keys[i].Status = "cooldown"
			until := time.Now().Add(time.Duration(rs.cfg.Cooldown.RateLimitSeconds) * time.Second)
			rs.keys[i].CooldownEnd = &until
			keyRecord = rs.keyRuntimeRecord(rs.keys[i])
			break
		}
	}
	rs.mu.Unlock()
	rs.persistKeyRuntime(keyRecord)
}

func (h *HealthChecker) markRecovered(rs *RouterService, keyID string) {
	var keyRecord *storage.KeyRuntimeRecord
	rs.mu.Lock()
	for i := range rs.keys {
		if rs.keys[i].ID == keyID {
			if rs.keys[i].Status == "invalid" || rs.keys[i].Status == "cooldown" {
				rs.keys[i].Status = "active"
				rs.keys[i].CooldownEnd = nil
				keyRecord = rs.keyRuntimeRecord(rs.keys[i])
			}
			break
		}
	}
	rs.mu.Unlock()
	rs.persistKeyRuntime(keyRecord)
}
