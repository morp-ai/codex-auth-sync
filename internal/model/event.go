package model

const (
	ProtocolVersion = 1

	KindAccountUpsert = "account_upsert"
	KindAccountDelete = "account_delete"
	KindAliasSet      = "alias_set"
	KindAccountStatus = "account_status"
	KindActiveSet     = "active_set"
	KindSettingsSet   = "settings_set"
)

type Event struct {
	Revision   int64  `json:"revision,omitempty"`
	EventID    string `json:"event_id"`
	ClusterID  string `json:"cluster_id"`
	NodeID     string `json:"node_id"`
	Kind       string `json:"kind"`
	EntityID   string `json:"entity_id"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  string `json:"created_at,omitempty"`
}

type EventsResponse struct {
	Events         []Event `json:"events"`
	LatestRevision int64   `json:"latest_revision"`
}

type HealthResponse struct {
	OK       bool   `json:"ok"`
	Service  string `json:"service"`
	Protocol int    `json:"protocol"`
}

func ValidKind(kind string) bool {
	switch kind {
	case KindAccountUpsert, KindAccountDelete, KindAliasSet, KindAccountStatus, KindActiveSet, KindSettingsSet:
		return true
	default:
		return false
	}
}
