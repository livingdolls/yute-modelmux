package domain

type RotationStrategy string

const (
	StrategyFailover   RotationStrategy = "failover"
	StrategyRoundRobin RotationStrategy = "round_robin"
	StrategyLeastError RotationStrategy = "least_error"
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
}
