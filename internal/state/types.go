package state

import "time"

type ApprovalPolicy string

const (
	ApprovalConservative ApprovalPolicy = "conservative"
	ApprovalAllowAll     ApprovalPolicy = "allow-all"
)

type SessionSummary struct {
	ID                string
	Title             string
	UpdatedAt         time.Time
	ProviderSessionID string
	Model             string
	Mode              string
	ApprovalPolicy    ApprovalPolicy
}

type ModelOption struct {
	ID                string
	Name              string
	SupportsReasoning bool
	DefaultReasoning  string
}

type TranscriptEntry struct {
	When   time.Time
	Role   string
	Kind   string
	ID     string
	Text   string
	Detail string
	Status string
}

type ActivityEntry struct {
	When   time.Time
	Kind   string
	Text   string
	Detail string
	Status string
}
