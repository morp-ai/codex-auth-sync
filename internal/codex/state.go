package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CurrentRegistrySchema = 4
	DefaultLiveInterval   = 60
)

var ErrUnsupportedRegistry = errors.New("unsupported codex-auth registry schema")

type Record struct {
	Fields map[string]json.RawMessage
}

type Registry struct {
	Root     map[string]json.RawMessage
	Accounts []*Record
}

type LocalAccount struct {
	AccountKey     string
	Alias          string
	Record         json.RawMessage
	Snapshot       []byte
	RecordHash     string
	AuthHash       string
	StatusHash     string
	AuthFreshness  int64
	StatusUnixTime int64
}

type LocalState struct {
	Accounts        map[string]LocalAccount
	AccountOrder    []string
	ActiveAccount   string
	IntervalSeconds int
}

func ReadLocalState(codexHome string) (LocalState, error) {
	_, registry, err := loadRegistry(codexHome)
	if err != nil {
		return LocalState{}, err
	}
	state := LocalState{
		Accounts:        make(map[string]LocalAccount),
		ActiveAccount:   registry.ActiveAccountKey(),
		IntervalSeconds: registry.IntervalSeconds(),
	}
	for _, record := range registry.Accounts {
		key, err := record.String("account_key")
		if err != nil || key == "" {
			continue
		}
		raw, err := record.Marshal()
		if err != nil {
			return LocalState{}, err
		}
		snapshot, err := os.ReadFile(AccountSnapshotPath(codexHome, key))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return LocalState{}, err
		}
		if errors.Is(err, os.ErrNotExist) {
			snapshot = nil
		}
		alias, _ := record.String("alias")
		account := LocalAccount{
			AccountKey:     key,
			Alias:          alias,
			Record:         raw,
			Snapshot:       snapshot,
			RecordHash:     hashRecord(record, false),
			AuthHash:       hashBytes(snapshot),
			StatusHash:     hashStatus(record),
			AuthFreshness:  AuthFreshness(snapshot),
			StatusUnixTime: record.Int64OrZero("last_usage_at"),
		}
		if account.StatusUnixTime == 0 {
			account.StatusUnixTime = record.Int64OrZero("last_used_at")
		}
		state.Accounts[key] = account
		state.AccountOrder = append(state.AccountOrder, key)
	}
	return state, nil
}

func AccountSnapshotFileName(accountKey string) string {
	key := accountKey
	if keyNeedsEncoding(key) {
		key = base64.RawURLEncoding.EncodeToString([]byte(key))
	}
	return key + ".auth.json"
}

func AccountSnapshotPath(codexHome, accountKey string) string {
	return filepath.Join(codexHome, "accounts", AccountSnapshotFileName(accountKey))
}

func ActiveAuthPath(codexHome string) string {
	return filepath.Join(codexHome, "auth.json")
}

func RegistryPath(codexHome string) string {
	return filepath.Join(codexHome, "accounts", "registry.json")
}

func ParseRecord(raw json.RawMessage) (*Record, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return &Record{Fields: fields}, nil
}

func (record *Record) Clone() *Record {
	fields := make(map[string]json.RawMessage, len(record.Fields))
	for key, value := range record.Fields {
		fields[key] = append(json.RawMessage(nil), value...)
	}
	return &Record{Fields: fields}
}

func (record *Record) Marshal() ([]byte, error) {
	return json.Marshal(record.Fields)
}

func (record *Record) String(name string) (string, error) {
	raw, ok := record.Fields[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func (record *Record) SetString(name, value string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	record.Fields[name] = raw
	return nil
}

func (record *Record) Int64OrZero(name string) int64 {
	raw, ok := record.Fields[name]
	if !ok {
		return 0
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0
	}
	return value
}

func (registry *Registry) ActiveAccountKey() string {
	raw, ok := registry.Root["active_account_key"]
	if !ok {
		return ""
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return ""
	}
	return *value
}

func (registry *Registry) IntervalSeconds() int {
	raw, ok := registry.Root["interval_seconds"]
	if !ok {
		return DefaultLiveInterval
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 5 || value > 3600 {
		return DefaultLiveInterval
	}
	return value
}

func (registry *Registry) Find(accountKey string) (int, *Record) {
	for index, record := range registry.Accounts {
		key, _ := record.String("account_key")
		if key == accountKey {
			return index, record
		}
	}
	return -1, nil
}

func (registry *Registry) Marshal() ([]byte, error) {
	accounts := make([]json.RawMessage, 0, len(registry.Accounts))
	for _, record := range registry.Accounts {
		raw, err := record.Marshal()
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, raw)
	}
	accountsRaw, err := json.Marshal(accounts)
	if err != nil {
		return nil, err
	}
	registry.Root["accounts"] = accountsRaw
	registry.Root["schema_version"] = json.RawMessage("4")
	data, err := json.MarshalIndent(registry.Root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func loadRegistry(codexHome string) ([]byte, *Registry, error) {
	path := RegistryPath(codexHome)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		registry := defaultRegistry()
		return nil, registry, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parse registry.json: %w", err)
	}
	var schema int
	if raw, ok := root["schema_version"]; ok {
		_ = json.Unmarshal(raw, &schema)
	}
	if schema > CurrentRegistrySchema {
		return nil, nil, fmt.Errorf("%w: schema_version=%d", ErrUnsupportedRegistry, schema)
	}
	if schema == 0 {
		return nil, nil, errors.New("registry.json is missing schema_version")
	}
	var accountMessages []json.RawMessage
	if raw, ok := root["accounts"]; ok {
		if err := json.Unmarshal(raw, &accountMessages); err != nil {
			return nil, nil, fmt.Errorf("parse registry accounts: %w", err)
		}
	}
	registry := &Registry{Root: root, Accounts: make([]*Record, 0, len(accountMessages))}
	for _, message := range accountMessages {
		record, err := ParseRecord(message)
		if err != nil {
			return nil, nil, fmt.Errorf("parse account record: %w", err)
		}
		registry.Accounts = append(registry.Accounts, record)
	}
	return data, registry, nil
}

func defaultRegistry() *Registry {
	return &Registry{
		Root: map[string]json.RawMessage{
			"schema_version":                 json.RawMessage("4"),
			"active_account_key":             json.RawMessage("null"),
			"previous_active_account_key":    json.RawMessage("null"),
			"active_account_activated_at_ms": json.RawMessage("null"),
			"interval_seconds":               json.RawMessage("60"),
			"accounts":                       json.RawMessage("[]"),
		},
		Accounts: []*Record{},
	}
}

func hashRecord(record *Record, includeAlias bool) string {
	copy := record.Clone()
	for _, key := range []string{"last_used_at", "last_usage", "last_usage_at", "last_local_rollout"} {
		delete(copy.Fields, key)
	}
	if !includeAlias {
		delete(copy.Fields, "alias")
	}
	raw, _ := copy.Marshal()
	return hashBytes(raw)
}

func hashStatus(record *Record) string {
	fields := make(map[string]json.RawMessage)
	for _, key := range []string{"last_used_at", "last_usage", "last_usage_at"} {
		if value, ok := record.Fields[key]; ok {
			fields[key] = value
		}
	}
	raw, _ := json.Marshal(fields)
	return hashBytes(raw)
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func keyNeedsEncoding(key string) bool {
	if key == "" || key == "." || key == ".." {
		return true
	}
	for _, char := range key {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return true
	}
	return false
}

func CanonicalAccountKeys(state LocalState) []string {
	keys := make([]string, 0, len(state.Accounts))
	for key := range state.Accounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ValidateAccountKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("account_key is required")
	}
	if len(key) > 4096 {
		return errors.New("account_key is too long")
	}
	return nil
}
