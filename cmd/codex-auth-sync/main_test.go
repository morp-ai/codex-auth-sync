package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNowIsRecognizedAsImmediateSyncAlias(t *testing.T) {
	missingConfig := filepath.Join(t.TempDir(), "missing.json")
	err := run([]string{"now", "--config", missingConfig})
	if err == nil {
		t.Fatal("now unexpectedly succeeded with a missing configuration")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("now was not recognized: %v", err)
	}
}
