package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestWebhookMatchesStatusFilters(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{
		"status_from": {"in_progress"},
		"status_to":   {"in_review"},
		"source":      {"task_completed"},
	})
	wh := db.WorkspaceWebhook{Enabled: true, Events: []byte(`["issue.status_changed"]`), Filters: filters}
	payload := map[string]any{
		"event_type": EventIssueStatusChanged,
		"transition": map[string]any{"from": "in_progress", "to": "in_review", "source": "task_completed"},
	}
	if !webhookMatches(wh, EventIssueStatusChanged, payload) {
		t.Fatal("expected webhook to match transition filters")
	}
	payload["transition"].(map[string]any)["to"] = "done"
	if webhookMatches(wh, EventIssueStatusChanged, payload) {
		t.Fatal("expected webhook not to match different to status")
	}
}

func TestWebhookMatchesTaskFailureFilters(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{
		"failure_reason": {"model_access", "model_not_found"},
		"will_retry":     {"false"},
	})
	wh := db.WorkspaceWebhook{Events: []byte(`["task.failed"]`), Filters: filters}
	payload := map[string]any{
		"event_type": EventTaskFailed,
		"task":       map[string]any{"failure_reason": "model_access", "will_retry": false},
	}
	if !webhookMatches(wh, EventTaskFailed, payload) {
		t.Fatal("expected webhook to match task failure filters")
	}
	payload["task"].(map[string]any)["will_retry"] = true
	if webhookMatches(wh, EventTaskFailed, payload) {
		t.Fatal("expected webhook not to match a retrying task")
	}
}

func TestWebhookDoesNotMatchIrrelevantFilter(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{"failure_reason": {"timeout"}})
	wh := db.WorkspaceWebhook{Events: []byte(`["issue.status_changed"]`), Filters: filters}
	payload := map[string]any{
		"event_type": EventIssueStatusChanged,
		"transition": map[string]any{"from": "todo", "to": "in_progress", "source": "manual_update"},
	}
	if webhookMatches(wh, EventIssueStatusChanged, payload) {
		t.Fatal("irrelevant filter key must not broaden matching")
	}
}

func TestWebhookMatchesMixedEventFilters(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{
		"status_to":      {"in_review"},
		"failure_reason": {"model_access"},
		"will_retry":     {"false"},
	})
	wh := db.WorkspaceWebhook{
		Events:  []byte(`["issue.status_changed","task.failed"]`),
		Filters: filters,
	}
	issuePayload := map[string]any{
		"event_type": EventIssueStatusChanged,
		"transition": map[string]any{"from": "in_progress", "to": "in_review", "source": "task_completed"},
	}
	if !webhookMatches(wh, EventIssueStatusChanged, issuePayload) {
		t.Fatal("expected issue event to ignore task filters selected for the same webhook")
	}
	taskPayload := map[string]any{
		"event_type": EventTaskFailed,
		"task":       map[string]any{"failure_reason": "model_access", "will_retry": false},
	}
	if !webhookMatches(wh, EventTaskFailed, taskPayload) {
		t.Fatal("expected task event to ignore issue filters selected for the same webhook")
	}
}

func TestWebhookDoesNotMatchDifferentEvent(t *testing.T) {
	wh := db.WorkspaceWebhook{Events: []byte(`["server.ready"]`)}
	if webhookMatches(wh, EventTaskFailed, map[string]any{"event_type": EventTaskFailed}) {
		t.Fatal("expected event subscription mismatch")
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

func TestSignGenericWebhookPayloadV2(t *testing.T) {
	payload := []byte(`{"ok":true}`)
	got := signGenericWebhookPayloadV2("secret", "123", payload)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("123"))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestSetGenericWebhookSignatureHeaders(t *testing.T) {
	payload := []byte(`{"ok":true}`)
	header := make(http.Header)
	setGenericWebhookSignatureHeaders(header, "secret", "123", payload)
	if got := header.Get("X-Webhook-Timestamp"); got != "123" {
		t.Fatalf("timestamp = %q, want 123", got)
	}
	if got := header.Get("X-Webhook-Signature"); got != signGenericWebhookPayload("secret", payload) {
		t.Fatalf("V1 signature = %q", got)
	}
	if got := header.Get("X-Webhook-Signature-V2"); got != signGenericWebhookPayloadV2("secret", "123", payload) {
		t.Fatalf("V2 signature = %q", got)
	}
}

func TestValidateWebhookURLRejectsRestrictedDestinations(t *testing.T) {
	for _, rawURL := range []string{
		"http://user:password@example.com/hook",
		"http://169.254.169.254/latest/meta-data",
		"http://[fd00:ec2::254]/latest/meta-data",
		"http://metadata.google.internal/computeMetadata/v1",
	} {
		if err := ValidateWebhookURL(rawURL); err == nil {
			t.Fatalf("expected restricted URL to be rejected: %s", rawURL)
		}
	}
	if err := ValidateWebhookURL("http://127.0.0.1:8645/webhooks/multica-ops"); err != nil {
		t.Fatalf("local self-hosted webhook should remain supported: %v", err)
	}
}

func TestWebhookHTTPClientDoesNotFollowRedirects(t *testing.T) {
	var reachedTarget atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reachedTarget.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	resp, err := newWebhookHTTPClient().Post(redirect.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || reachedTarget.Load() {
		t.Fatalf("redirect was followed: status=%d target=%v", resp.StatusCode, reachedTarget.Load())
	}
}
