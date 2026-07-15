package hub

import (
	"path/filepath"
	"testing"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

func TestStoreAppendListPersistAndDeduplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	input := model.Event{
		EventID:    "00112233445566778899aabbccddeeff",
		ClusterID:  "cluster-a",
		NodeID:     "node-a",
		Kind:       model.KindActiveSet,
		EntityID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Ciphertext: "ciphertext",
	}
	first, err := store.Append(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", first.Revision)
	}
	duplicate, err := store.Append(input)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Revision != first.Revision || store.EventCount() != 1 {
		t.Fatal("duplicate event must not be appended")
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	events, latest, _ := reopened.List("cluster-a", 0, 10)
	if latest != 1 || len(events) != 1 || events[0].EventID != input.EventID {
		t.Fatalf("unexpected persisted events: latest=%d events=%v", latest, events)
	}
}
