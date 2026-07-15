package model

import "encoding/json"

type AccountUpsertPayload struct {
	AccountKey    string          `json:"account_key"`
	Record        json.RawMessage `json:"record"`
	Snapshot      string          `json:"snapshot_base64"`
	AuthFreshness int64           `json:"auth_freshness"`
}

type AccountDeletePayload struct {
	AccountKey string `json:"account_key"`
}

type AliasSetPayload struct {
	AccountKey string `json:"account_key"`
	Alias      string `json:"alias"`
}

type AccountStatusPayload struct {
	AccountKey     string          `json:"account_key"`
	LastUsedAt     json.RawMessage `json:"last_used_at"`
	LastUsage      json.RawMessage `json:"last_usage"`
	LastUsageAt    json.RawMessage `json:"last_usage_at"`
	StatusUnixTime int64           `json:"status_unix_time"`
}

type ActiveSetPayload struct {
	AccountKey string `json:"account_key,omitempty"`
	Group      string `json:"group"`
}

type SettingsSetPayload struct {
	IntervalSeconds int `json:"interval_seconds"`
}
