package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type webhookResponse struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	URL       string              `json:"url"`
	Enabled   bool                `json:"enabled"`
	Events    []string            `json:"events"`
	Filters   map[string][]string `json:"filters"`
	Secret    *string             `json:"secret,omitempty"`
	CreatedAt string              `json:"created_at"`
	UpdatedAt string              `json:"updated_at"`
}

type webhookDeliveryResponse struct {
	ID             string          `json:"id"`
	WebhookID      string          `json:"webhook_id"`
	EventType      string          `json:"event_type"`
	EventID        string          `json:"event_id"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	AttemptCount   int32           `json:"attempt_count"`
	NextAttemptAt  string          `json:"next_attempt_at"`
	LastAttemptAt  *string         `json:"last_attempt_at"`
	ResponseStatus *int32          `json:"response_status"`
	ResponseBody   *string         `json:"response_body"`
	Error          *string         `json:"error"`
	CreatedAt      string          `json:"created_at"`
	DeliveredAt    *string         `json:"delivered_at"`
}

type webhookRequest struct {
	Name    string              `json:"name"`
	URL     string              `json:"url"`
	Enabled *bool               `json:"enabled"`
	Events  []string            `json:"events"`
	Filters map[string][]string `json:"filters"`
}

func webhookToResponse(w db.WorkspaceWebhook, includeSecret bool) webhookResponse {
	events := []string{service.EventIssueStatusChanged}
	if len(w.Events) > 0 {
		json.Unmarshal(w.Events, &events)
	}
	filters := map[string][]string{}
	if len(w.Filters) > 0 {
		json.Unmarshal(w.Filters, &filters)
	}
	resp := webhookResponse{
		ID:        uuidToString(w.ID),
		Name:      w.Name,
		URL:       w.Url,
		Enabled:   w.Enabled,
		Events:    events,
		Filters:   filters,
		CreatedAt: timestampToString(w.CreatedAt),
		UpdatedAt: timestampToString(w.UpdatedAt),
	}
	if includeSecret {
		resp.Secret = &w.Secret
	}
	return resp
}

func webhookDeliveryToResponse(d db.WebhookDelivery) webhookDeliveryResponse {
	var responseStatus *int32
	if d.ResponseStatus.Valid {
		v := d.ResponseStatus.Int32
		responseStatus = &v
	}
	return webhookDeliveryResponse{
		ID:             uuidToString(d.ID),
		WebhookID:      uuidToString(d.WebhookID),
		EventType:      d.EventType,
		EventID:        uuidToString(d.EventID),
		Payload:        json.RawMessage(d.Payload),
		Status:         d.Status,
		AttemptCount:   d.AttemptCount,
		NextAttemptAt:  timestampToString(d.NextAttemptAt),
		LastAttemptAt:  timestampToPtr(d.LastAttemptAt),
		ResponseStatus: responseStatus,
		ResponseBody:   textToPtr(d.ResponseBody),
		Error:          textToPtr(d.Error),
		CreatedAt:      timestampToString(d.CreatedAt),
		DeliveredAt:    timestampToPtr(d.DeliveredAt),
	}
}

func normalizeWebhookRequest(req webhookRequest) ([]byte, []byte, bool, error) {
	events := req.Events
	if len(events) == 0 {
		events = []string{service.EventIssueStatusChanged}
	}
	for _, event := range events {
		if !service.IsSupportedWebhookEvent(event) {
			return nil, nil, false, errInvalidWebhookEvent
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	filters := req.Filters
	if filters == nil {
		filters = map[string][]string{}
	}
	eventsJSON, _ := json.Marshal(events)
	filtersJSON, _ := json.Marshal(filters)
	return eventsJSON, filtersJSON, enabled, nil
}

func normalizeWebhookUpdateRequest(req webhookRequest) ([]byte, []byte, error) {
	var eventsJSON []byte
	if req.Events != nil {
		if len(req.Events) == 0 {
			return nil, nil, errWebhookEventsRequired
		}
		for _, event := range req.Events {
			if !service.IsSupportedWebhookEvent(event) {
				return nil, nil, errInvalidWebhookEvent
			}
		}
		eventsJSON, _ = json.Marshal(req.Events)
	}
	var filtersJSON []byte
	if req.Filters != nil {
		filtersJSON, _ = json.Marshal(req.Filters)
	}
	return eventsJSON, filtersJSON, nil
}

var errInvalidWebhookEvent = &webhookValidationError{"unsupported webhook event"}
var errWebhookEventsRequired = &webhookValidationError{"at least one webhook event is required"}

type webhookValidationError struct{ msg string }

func (e *webhookValidationError) Error() string { return e.msg }

func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	webhooks, err := h.Queries.ListWorkspaceWebhooks(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}
	resp := make([]webhookResponse, 0, len(webhooks))
	for _, wh := range webhooks {
		resp = append(resp, webhookToResponse(wh, false))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := service.ValidateWebhookURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	eventsJSON, filtersJSON, enabled, err := normalizeWebhookRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	secret, err := service.GenerateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret")
		return
	}
	webhook, err := h.Queries.CreateWorkspaceWebhook(r.Context(), db.CreateWorkspaceWebhookParams{
		WorkspaceID: parseUUID(workspaceID),
		Name:        req.Name,
		Url:         req.URL,
		Secret:      secret,
		Enabled:     enabled,
		Events:      eventsJSON,
		Filters:     filtersJSON,
		CreatedBy:   parseUUID(userID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}
	writeJSON(w, http.StatusCreated, webhookToResponse(webhook, true))
}

func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	webhook, err := h.Queries.GetWorkspaceWebhook(r.Context(), db.GetWorkspaceWebhookParams{ID: id, WorkspaceID: parseUUID(workspaceID)})
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, webhookToResponse(webhook, false))
}

func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL != "" {
		if err := service.ValidateWebhookURL(req.URL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	eventsJSON, filtersJSON, err := normalizeWebhookUpdateRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	params := db.UpdateWorkspaceWebhookParams{ID: id, WorkspaceID: parseUUID(workspaceID), Events: eventsJSON, Filters: filtersJSON}
	if strings.TrimSpace(req.Name) != "" {
		params.Name = pgtype.Text{String: strings.TrimSpace(req.Name), Valid: true}
	}
	if strings.TrimSpace(req.URL) != "" {
		params.Url = pgtype.Text{String: strings.TrimSpace(req.URL), Valid: true}
	}
	if req.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *req.Enabled, Valid: true}
	}
	webhook, err := h.Queries.UpdateWorkspaceWebhook(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, webhookToResponse(webhook, false))
}

func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	if err := h.Queries.DeleteWorkspaceWebhook(r.Context(), db.DeleteWorkspaceWebhookParams{ID: id, WorkspaceID: parseUUID(workspaceID)}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	secret, err := service.GenerateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret")
		return
	}
	webhook, err := h.Queries.RotateWorkspaceWebhookSecret(r.Context(), db.RotateWorkspaceWebhookSecretParams{
		ID: id, WorkspaceID: parseUUID(workspaceID), Secret: secret,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, webhookToResponse(webhook, true))
}

func (h *Handler) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	deliveries, err := h.Queries.ListWebhookDeliveries(r.Context(), db.ListWebhookDeliveriesParams{
		WebhookID: id, WorkspaceID: parseUUID(workspaceID), Limit: 50,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}
	resp := make([]webhookDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		resp = append(resp, webhookDeliveryToResponse(d))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RetryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	d, err := h.Queries.RetryWebhookDelivery(r.Context(), db.RetryWebhookDeliveryParams{ID: id, WorkspaceID: parseUUID(workspaceID)})
	if err != nil {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	writeJSON(w, http.StatusOK, webhookDeliveryToResponse(d))
}

func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	webhook, err := h.Queries.GetWorkspaceWebhook(r.Context(), db.GetWorkspaceWebhookParams{ID: id, WorkspaceID: parseUUID(workspaceID)})
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load webhook")
		return
	}
	eventUUID := uuid.Must(uuid.NewRandom()).String()
	eventID := parseUUID(eventUUID)
	eventType := firstWebhookEvent(webhook.Events)
	payload := testWebhookPayload(workspaceID, eventUUID, eventType)
	raw, _ := json.Marshal(payload)
	delivery, err := h.Queries.CreateWebhookDelivery(r.Context(), db.CreateWebhookDeliveryParams{
		WebhookID: webhook.ID, WorkspaceID: webhook.WorkspaceID, EventType: eventType,
		EventID: eventID, Payload: raw,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create test delivery")
		return
	}
	if h.WebhookService != nil {
		go h.WebhookService.DispatchDelivery(context.Background(), delivery)
	}
	writeJSON(w, http.StatusAccepted, webhookDeliveryToResponse(delivery))
}

func firstWebhookEvent(raw []byte) string {
	var events []string
	if json.Unmarshal(raw, &events) == nil {
		for _, eventType := range events {
			if service.IsSupportedWebhookEvent(eventType) {
				return eventType
			}
		}
	}
	return service.EventIssueStatusChanged
}

func testWebhookPayload(workspaceID, eventID, eventType string) map[string]any {
	now := timeNowRFC3339()
	payload := map[string]any{
		"schema_version": 1,
		"event":          eventType,
		"event_type":     eventType,
		"event_id":       eventID,
		"occurred_at":    now,
		"workspace":      map[string]any{"id": workspaceID, "slug": "test", "name": "Test workspace"},
	}
	switch eventType {
	case service.EventTaskFailed:
		payload["issue"] = map[string]any{"id": eventID, "identifier": "TEST-1", "title": "Test webhook", "status": "todo"}
		payload["task"] = map[string]any{
			"id": eventID, "agent_id": eventID, "runtime_id": eventID,
			"attempt": 1, "max_attempts": 1, "failure_reason": "agent_error",
			"error": "Test delivery", "will_retry": false, "retry_kind": "none",
			"retry_task_id": nil, "completed_at": now,
		}
	case service.EventRuntimeOffline:
		payload["runtime"] = map[string]any{"id": eventID, "provider": "test"}
		payload["failed_task_count"] = 0
		payload["affected_issue_count"] = 0
		payload["affected_issues"] = []any{}
	case service.EventRuntimeRecovered:
		payload["runtime"] = map[string]any{"id": eventID, "provider": "test", "boot_id": eventID}
		payload["orphan_count"] = 0
		payload["retried_count"] = 0
		payload["final_failure_count"] = 0
		payload["affected_issue_count"] = 0
		payload["affected_issues"] = []any{}
	case service.EventServerReady:
		payload["server"] = map[string]any{
			"boot_id": eventID, "version": "test", "bind_address": "127.0.0.1:0", "started_at": now,
		}
	default:
		payload["issue"] = map[string]any{"id": eventID, "identifier": "TEST-1", "title": "Test webhook"}
		payload["transition"] = map[string]any{"from": "in_progress", "to": "in_review", "source": "test", "occurred_at": now}
	}
	return payload
}

func timeNowRFC3339() string {
	return timestampToString(pgtype.Timestamptz{Time: nowUTC(), Valid: true})
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
