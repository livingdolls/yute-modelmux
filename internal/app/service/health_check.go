package service

import (
	"context"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

type HealthChecker struct {
	rs     *RouterService
	cfg    config.HealthCheckConfig
	cancel context.CancelFunc
}

func NewHealthChecker(rs *RouterService, cfg config.HealthCheckConfig) *HealthChecker {
	return &HealthChecker{rs: rs, cfg: cfg}
}

func (h *HealthChecker) Start(parentCtx context.Context) {
	if !h.cfg.Enabled || h.cfg.IntervalSeconds <= 0 {
		return
	}
	interval := time.Duration(h.cfg.IntervalSeconds) * time.Second
	ctx, cancel := context.WithCancel(parentCtx)
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

func (h *HealthChecker) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *HealthChecker) runCheck(ctx context.Context) {
	keys := h.rs.ListKeys()
	timeout := time.Duration(h.cfg.TimeoutSeconds) * time.Second
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

		err := h.rs.TestKey(checkCtx, key.ID)
		if err != nil {
			h.rs.mu.Lock()
			for i := range h.rs.keys {
				if h.rs.keys[i].ID == key.ID {
					if h.rs.keys[i].Status != "disabled" && h.rs.keys[i].Status != "invalid" {
						h.rs.keys[i].Status = "invalid"
						h.rs.saveKeyRuntimeLocked(h.rs.keys[i])
					}
					break
				}
			}
			h.rs.mu.Unlock()
		} else {
			h.rs.mu.Lock()
			for i := range h.rs.keys {
				if h.rs.keys[i].ID == key.ID {
					if h.rs.keys[i].Status == "invalid" {
						h.rs.keys[i].Status = "active"
						h.rs.keys[i].CooldownEnd = nil
						h.rs.saveKeyRuntimeLocked(h.rs.keys[i])
					}
					break
				}
			}
			h.rs.mu.Unlock()
		}
	}
}
