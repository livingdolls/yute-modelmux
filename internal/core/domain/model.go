package domain

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
	Capabilities          Capabilities
}

type Capabilities struct {
	Chat        bool
	Completions bool
	Streaming   bool
	Tools       bool
	Vision      bool
	JSONMode    bool
}
