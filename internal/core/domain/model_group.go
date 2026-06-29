package domain

type GroupStrategy string

const (
	GroupStrategyFailover   GroupStrategy = "failover"
	GroupStrategyRoundRobin GroupStrategy = "round_robin"
	GroupStrategyWeighted   GroupStrategy = "weighted"
)

type ModelGroup struct {
	ID       string
	Name     string
	Strategy GroupStrategy
	Enabled  bool
	Members  []ModelGroupMember
}

type ModelGroupMember struct {
	ModelID  string
	KeyID    string
	Priority int
	Weight   int
	Enabled  bool
}
