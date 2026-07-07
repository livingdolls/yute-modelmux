package domain

import "time"

type RotationStrategy string

const (
	StrategyFailover   RotationStrategy = "failover"
	StrategyRoundRobin RotationStrategy = "round_robin"
	StrategyLeastError RotationStrategy = "least_error"
	StrategyLeastUsed  RotationStrategy = "least_used"
)

type Model struct {
	ID                    string
	ProviderID            string
	ModelName             string
	Strategy              RotationStrategy
	Enabled               bool
	RequestsPerMinute     int
	MaxConcurrentRequests int
	ConcurrentCount       int
	MinuteWindowStart     time.Time
	MinuteRequestCount    int
	Capabilities          Capabilities
	Cost                  CostConfig
}

type CostConfig struct {
	InputPer1M  float64
	OutputPer1M float64
}

type Capabilities struct {
	Chat        bool
	Completions bool
	Streaming   bool
	Tools       bool
	Vision      bool
	JSONMode    bool
}
