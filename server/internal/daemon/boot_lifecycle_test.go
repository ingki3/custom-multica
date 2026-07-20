package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestDaemonBootIDStableAndSentDuringRecovery(t *testing.T) {
	var receivedBootID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BootID string `json:"boot_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode recovery request: %v", err)
		}
		receivedBootID = body.BootID
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{ServerBaseURL: srv.URL, WorkspacesRoot: t.TempDir()}, slog.Default())
	if uuid.Validate(d.bootID) != nil {
		t.Fatalf("daemon boot ID is not a UUID: %q", d.bootID)
	}
	stableBootID := d.bootID
	if err := d.client.RecoverOrphans(context.Background(), uuid.NewString(), d.bootID); err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if d.bootID != stableBootID {
		t.Fatal("daemon boot ID changed during process lifetime")
	}
	if receivedBootID != stableBootID {
		t.Fatalf("sent boot ID %q, want %q", receivedBootID, stableBootID)
	}
	if other := New(Config{ServerBaseURL: srv.URL, WorkspacesRoot: t.TempDir()}, slog.Default()).bootID; other == stableBootID {
		t.Fatal("new daemon process instance should receive a distinct boot ID")
	}
}
