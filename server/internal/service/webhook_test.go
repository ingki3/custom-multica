package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestWebhookMatchesStatusFilters(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{
		"status_from": []string{"in_progress"},
		"status_to":   []string{"in_review"},
		"source":      []string{"task_completed"},
	})
	wh := db.WorkspaceWebhook{
		Enabled: true,
		Events:  []byte(`["issue.status_changed"]`),
		Filters: filters,
	}
	payload := map[string]any{
		"transition": map[string]any{
			"from":   "in_progress",
			"to":     "in_review",
			"source": "task_completed",
		},
	}
	if !webhookMatches(wh, payload) {
		t.Fatal("expected webhook to match transition filters")
	}
	payload["transition"].(map[string]any)["to"] = "done"
	if webhookMatches(wh, payload) {
		t.Fatal("expected webhook not to match different to status")
	}
}

func TestSignWebhookPayload(t *testing.T) {
	payload := []byte(`{"event":"issue.status_changed"}`)
	got := signWebhookPayload("secret", "123", payload)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("123"))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestSignGenericWebhookPayload(t *testing.T) {
	payload := []byte(`{"event":"issue.status_changed"}`)
	got := signGenericWebhookPayload("secret", payload)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}
