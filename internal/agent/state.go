package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type ObservedAccount struct {
	RecordHash string `json:"record_hash"`
	AuthHash   string `json:"auth_hash"`
	Alias      string `json:"alias"`
	StatusHash string `json:"status_hash"`
}

type PersistentState struct {
	Version          int                        `json:"version"`
	Initialized      bool                       `json:"initialized"`
	LastRevision     int64                      `json:"last_revision"`
	Accounts         map[string]ObservedAccount `json:"accounts"`
	ActiveAccount    string                     `json:"active_account"`
	SettingsHash     string                     `json:"settings_hash"`
	LastSuccessfulAt string                     `json:"last_successful_at,omitempty"`
}

func defaultState() PersistentState {
	return PersistentState{Version: 1, Accounts: make(map[string]ObservedAccount)}
}

func loadState(path string) (PersistentState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultState(), nil
	}
	if err != nil {
		return PersistentState{}, err
	}
	state := defaultState()
	if err := json.Unmarshal(data, &state); err != nil {
		return PersistentState{}, err
	}
	if state.Version != 1 {
		return PersistentState{}, errors.New("unsupported agent state version")
	}
	if state.Accounts == nil {
		state.Accounts = make(map[string]ObservedAccount)
	}
	return state, nil
}

func saveState(path string, state PersistentState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil && runtime.GOOS != "windows" {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0600)
	}
	return nil
}
