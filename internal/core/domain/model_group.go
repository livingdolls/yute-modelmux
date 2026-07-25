package domain

type GroupStrategy string

const (
	GroupStrategyFailover       GroupStrategy = "failover"
	GroupStrategyRoundRobin     GroupStrategy = "round_robin"
	GroupStrategyWeighted       GroupStrategy = "weighted"
	GroupStrategyConsistentHash GroupStrategy = "consistent_hash"
)

type ModelGroup struct {
	ID                   string
	Name                 string
	Description          string
	Strategy             GroupStrategy
	Enabled              bool
	RequiredCapabilities []string
	ContextWindow        int
	MaxOutputTokens      int
	Capabilities         Capabilities
	Members              []ModelGroupMember
}

type ModelGroupMember struct {
	ModelID  string
	KeyID    string
	Priority int
	Weight   int
	Enabled  bool
}
