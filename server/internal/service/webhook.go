package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type StatusTransitionActor struct {
	Type string
	ID   pgtype.UUID
}

type StatusTransitionOptions struct {
	Source   string
	Actor    StatusTransitionActor
	TaskID   pgtype.UUID
	Metadata map[string]any
}

type WebhookService struct {
	Queries *db.Queries
	Bus     *events.Bus
	Client  *http.Client

	AppURL string
	APIURL string
}

func NewWebhookService(q *db.Queries, bus *events.Bus) *WebhookService {
	s := &WebhookService{
		Queries: q,
		Bus:     bus,
		Client:  newWebhookHTTPClient(),
		AppURL:  firstNonEmpty(os.Getenv("MULTICA_APP_URL"), os.Getenv("FRONTEND_ORIGIN"), "http://localhost:3000"),
		APIURL:  firstNonEmpty(os.Getenv("NEXT_PUBLIC_API_URL"), "http://localhost:8080"),
	}
	if bus != nil {
		for _, eventType := range SupportedWebhookEvents() {
			bus.Subscribe(eventType, s.EnqueueMatchingDeliveries)
		}
	}
	return s
}

func newWebhookHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Connect directly so proxy settings cannot bypass destination checks.
	transport.Proxy = nil
	transport.DialContext = webhookDialContext
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Redirects could pivot an approved URL to a restricted destination.
			return http.ErrUseLastResponse
		},
	}
}

func webhookDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("webhook host resolved to no addresses")
	}
	for _, resolved := range addresses {
		if forbiddenWebhookIP(resolved.IP) {
			return nil, fmt.Errorf("webhook destination is not allowed")
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func forbiddenWebhookIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.Equal(net.ParseIP("fd00:ec2::254"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	return ""
}

func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

func ValidateWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("url must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("url host is required")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("url hostname is required")
	}
	if u.User != nil {
		return fmt.Errorf("url credentials are not allowed")
	}
	hostname := u.Hostname()
	literalIP := net.ParseIP(hostname)
	if strings.EqualFold(hostname, "metadata.google.internal") ||
		strings.EqualFold(hostname, "metadata.google") ||
		(literalIP != nil && forbiddenWebhookIP(literalIP)) {
		return fmt.Errorf("webhook destination is not allowed")
	}
	return nil
}

func (s *WebhookService) TransitionIssueStatus(ctx context.Context, issueID pgtype.UUID, toStatus string, opts StatusTransitionOptions) (db.Issue, bool, error) {
	issue, err := s.Queries.GetIssue(ctx, issueID)
	if err != nil {
		return db.Issue{}, false, err
	}
	if issue.Status == toStatus {
		return issue, false, nil
	}
	fromStatus := issue.Status
	updated, err := s.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
		ID:     issueID,
		Status: toStatus,
	})
	if err != nil {
		return db.Issue{}, false, err
	}
	if err := s.RecordIssueStatusTransition(ctx, updated, fromStatus, opts); err != nil {
		slog.Warn("record issue status transition failed", "issue_id", util.UUIDToString(issueID), "error", err)
	}
	return updated, true, nil
}

func (s *WebhookService) RecordIssueStatusTransition(ctx context.Context, issue db.Issue, fromStatus string, opts StatusTransitionOptions) error {
	if fromStatus == issue.Status {
		return nil
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "manual_update"
	}
	actorType := strings.TrimSpace(opts.Actor.Type)
	if actorType == "" {
		actorType = "system"
	}
	meta := []byte("{}")
	if opts.Metadata != nil {
		if b, err := json.Marshal(opts.Metadata); err == nil {
			meta = b
		}
	}
	transition, err := s.Queries.CreateIssueStatusTransition(ctx, db.CreateIssueStatusTransitionParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		FromStatus:  fromStatus,
		ToStatus:    issue.Status,
		Source:      source,
		ActorType:   actorType,
		ActorID:     opts.Actor.ID,
		TaskID:      opts.TaskID,
		Metadata:    meta,
	})
	if err != nil {
		return err
	}
	payload := s.BuildStatusChangedPayload(ctx, transition, issue)
	if s.Bus != nil {
		s.Bus.Publish(events.Event{
			Type:        EventIssueStatusChanged,
			WorkspaceID: util.UUIDToString(issue.WorkspaceID),
			ActorType:   actorType,
			ActorID:     util.UUIDToString(opts.Actor.ID),
			TaskID:      util.UUIDToString(opts.TaskID),
			Payload:     payload,
		})
	}
	return nil
}

func (s *WebhookService) BuildStatusChangedPayload(ctx context.Context, tr db.IssueStatusTransition, issue db.Issue) map[string]any {
	ws, _ := s.Queries.GetWorkspace(ctx, tr.WorkspaceID)
	prefix := ws.IssuePrefix
	if prefix == "" {
		prefix = strings.ToUpper(ws.Slug)
	}
	identifier := fmt.Sprintf("%s-%d", prefix, issue.Number)
	workspaceSlug := ws.Slug
	issuePath := fmt.Sprintf("/%s/issues/%s", workspaceSlug, identifier)

	payload := map[string]any{
		"schema_version": 1,
		"event":          EventIssueStatusChanged,
		"event_type":     EventIssueStatusChanged,
		"event_id":       util.UUIDToString(tr.ID),
		"occurred_at":    tr.CreatedAt.Time.Format(time.RFC3339),
		"workspace": map[string]any{
			"id":   util.UUIDToString(tr.WorkspaceID),
			"slug": workspaceSlug,
			"name": ws.Name,
		},
		"issue": map[string]any{
			"id":         util.UUIDToString(issue.ID),
			"identifier": identifier,
			"number":     issue.Number,
			"title":      issue.Title,
			"status":     issue.Status,
			"url":        s.AppURL + issuePath,
			"api_url":    s.APIURL + "/api/issues/" + util.UUIDToString(issue.ID),
		},
		"transition": map[string]any{
			"from":        tr.FromStatus,
			"to":          tr.ToStatus,
			"source":      tr.Source,
			"occurred_at": tr.CreatedAt.Time.Format(time.RFC3339),
		},
		"actor": map[string]any{
			"type": tr.ActorType,
			"id":   uuidStringOrNil(tr.ActorID),
		},
	}
	if tr.TaskID.Valid {
		if task, err := s.Queries.GetAgentTask(ctx, tr.TaskID); err == nil {
			payload["task"] = map[string]any{
				"id":           util.UUIDToString(task.ID),
				"agent_id":     util.UUIDToString(task.AgentID),
				"runtime_id":   util.UUIDToString(task.RuntimeID),
				"completed_at": timestampStringOrNil(task.CompletedAt),
			}
		}
	}
	if issue.AssigneeID.Valid {
		assignee := map[string]any{
			"type": issue.AssigneeType.String,
			"id":   util.UUIDToString(issue.AssigneeID),
		}
		switch issue.AssigneeType.String {
		case "agent":
			if agent, err := s.Queries.GetAgent(ctx, issue.AssigneeID); err == nil {
				assignee["name"] = agent.Name
			}
		case "member":
			if member, err := s.Queries.GetMember(ctx, issue.AssigneeID); err == nil {
				if user, err := s.Queries.GetUser(ctx, member.UserID); err == nil {
					assignee["name"] = user.Name
				}
			}
		}
		payload["assignee"] = assignee
	}
	if issue.ProjectID.Valid {
		if project, err := s.Queries.GetProject(ctx, issue.ProjectID); err == nil {
			payload["project"] = map[string]any{
				"id":    util.UUIDToString(project.ID),
				"title": project.Title,
			}
		}
	}
	return payload
}

func uuidStringOrNil(u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	return util.UUIDToString(u)
}

func timestampStringOrNil(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.Format(time.RFC3339)
}

func (s *WebhookService) EnqueueMatchingDeliveries(e events.Event) {
	ctx := context.Background()
	payload, ok := e.Payload.(map[string]any)
	if !ok {
		return
	}
	if err := ValidateWebhookEnvelope(payload); err != nil {
		slog.Warn("invalid webhook event envelope", "event_type", e.Type, "error", err)
		return
	}
	eventType := payload["event_type"].(string)
	if eventType != e.Type {
		return
	}
	workspaceID, err := util.ParseUUID(e.WorkspaceID)
	if err != nil {
		return
	}
	webhooks, err := s.Queries.ListEnabledWorkspaceWebhooks(ctx, workspaceID)
	if err != nil {
		slog.Warn("list enabled webhooks failed", "workspace_id", e.WorkspaceID, "error", err)
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	eventID, err := util.ParseUUID(fmt.Sprint(payload["event_id"]))
	if err != nil {
		return
	}
	for _, wh := range webhooks {
		if !webhookMatches(wh, eventType, payload) {
			continue
		}
		if _, err := s.Queries.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
			WebhookID:   wh.ID,
			WorkspaceID: wh.WorkspaceID,
			EventType:   eventType,
			EventID:     eventID,
			Payload:     raw,
		}); err != nil {
			slog.Warn("create webhook delivery failed", "webhook_id", util.UUIDToString(wh.ID), "error", err)
		}
	}
}

var webhookFilterEvents = map[string]string{
	"status_from":    EventIssueStatusChanged,
	"status_to":      EventIssueStatusChanged,
	"source":         EventIssueStatusChanged,
	"failure_reason": EventTaskFailed,
	"will_retry":     EventTaskFailed,
}

func webhookMatches(wh db.WorkspaceWebhook, eventType string, payload map[string]any) bool {
	if fmt.Sprint(payload["event_type"]) != eventType {
		return false
	}
	subscribedEvents := []string{EventIssueStatusChanged}
	if len(wh.Events) > 0 {
		if err := json.Unmarshal(wh.Events, &subscribedEvents); err != nil {
			return false
		}
		if len(subscribedEvents) == 0 {
			if eventType != EventIssueStatusChanged {
				return false
			}
		} else if !contains(subscribedEvents, eventType) {
			return false
		}
	} else if eventType != EventIssueStatusChanged {
		// Webhooks created before event selection existed default to issue changes.
		return false
	}
	if len(wh.Filters) == 0 {
		return true
	}
	var filters map[string][]string
	if err := json.Unmarshal(wh.Filters, &filters); err != nil {
		return false
	}

	var values map[string]any
	var allowedKeys map[string]bool
	switch eventType {
	case EventIssueStatusChanged:
		transition, ok := payload["transition"].(map[string]any)
		if !ok {
			return false
		}
		values = map[string]any{
			"status_from": transition["from"],
			"status_to":   transition["to"],
			"source":      transition["source"],
		}
		allowedKeys = map[string]bool{"status_from": true, "status_to": true, "source": true}
	case EventTaskFailed:
		task, ok := payload["task"].(map[string]any)
		if !ok {
			return false
		}
		values = map[string]any{
			"failure_reason": task["failure_reason"],
			"will_retry":     task["will_retry"],
		}
		allowedKeys = map[string]bool{"failure_reason": true, "will_retry": true}
	default:
		allowedKeys = map[string]bool{}
	}
	for key, accepted := range filters {
		if !allowedKeys[key] {
			filterEvent, known := webhookFilterEvents[key]
			if !known || !contains(subscribedEvents, filterEvent) {
				return false
			}
			// A flat filter object stores filters for every selected event. Ignore
			// recognized filters belonging to another event in this subscription.
			continue
		}
		if len(accepted) > 0 && !contains(accepted, fmt.Sprint(values[key])) {
			return false
		}
	}
	return true
}

func contains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

func (s *WebhookService) StartDispatcher(ctx context.Context) {
	if os.Getenv("MULTICA_DISABLE_WEBHOOK_DISPATCHER") == "true" {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			s.DispatchDue(ctx, 10)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *WebhookService) DispatchDue(ctx context.Context, limit int32) {
	deliveries, err := s.Queries.ListDueWebhookDeliveries(ctx, limit)
	if err != nil {
		if !strings.Contains(err.Error(), "does not exist") {
			slog.Warn("list due webhook deliveries failed", "error", err)
		}
		return
	}
	for _, d := range deliveries {
		s.DispatchDelivery(ctx, d)
	}
}

func (s *WebhookService) DispatchDelivery(ctx context.Context, d db.WebhookDelivery) {
	wh, err := s.Queries.GetWorkspaceWebhook(ctx, db.GetWorkspaceWebhookParams{ID: d.WebhookID, WorkspaceID: d.WorkspaceID})
	if err != nil {
		if err != pgx.ErrNoRows {
			slog.Warn("load webhook for delivery failed", "delivery_id", util.UUIDToString(d.ID), "error", err)
		}
		return
	}
	if err := ValidateWebhookURL(wh.Url); err != nil {
		s.markFailedOrRetry(ctx, d, 0, "", err.Error())
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.Url, bytes.NewReader(d.Payload))
	if err != nil {
		s.markFailedOrRetry(ctx, d, 0, "", err.Error())
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := signWebhookPayload(wh.Secret, ts, d.Payload)
	deliveryID := util.UUIDToString(d.ID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Multica-Webhooks/1.0")
	req.Header.Set("X-Multica-Event", d.EventType)
	req.Header.Set("X-Multica-Delivery", deliveryID)
	req.Header.Set("X-Request-ID", deliveryID)
	req.Header.Set("X-Multica-Timestamp", ts)
	req.Header.Set("X-Multica-Signature", sig)
	setGenericWebhookSignatureHeaders(req.Header, wh.Secret, ts, d.Payload)

	resp, err := s.Client.Do(req)
	if err != nil {
		s.markFailedOrRetry(ctx, d, 0, "", err.Error())
		return
	}
	defer resp.Body.Close()
	body := readLimited(resp.Body, 4096)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if _, err := s.Queries.MarkWebhookDeliveryDelivered(ctx, db.MarkWebhookDeliveryDeliveredParams{
			ID:             d.ID,
			ResponseStatus: pgtype.Int4{Int32: int32(resp.StatusCode), Valid: true},
			ResponseBody:   pgtype.Text{String: body, Valid: body != ""},
		}); err != nil {
			slog.Warn("mark webhook delivery delivered failed", "delivery_id", util.UUIDToString(d.ID), "error", err)
		}
		return
	}
	s.markFailedOrRetry(ctx, d, resp.StatusCode, body, fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode))
}

func signWebhookPayload(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func signGenericWebhookPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func signGenericWebhookPayloadV2(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func setGenericWebhookSignatureHeaders(header http.Header, secret, timestamp string, payload []byte) {
	header.Set("X-Webhook-Signature", signGenericWebhookPayload(secret, payload))
	header.Set("X-Webhook-Timestamp", timestamp)
	header.Set("X-Webhook-Signature-V2", signGenericWebhookPayloadV2(secret, timestamp, payload))
}

func readLimited(r io.Reader, limit int64) string {
	b, _ := io.ReadAll(io.LimitReader(r, limit))
	return string(b)
}

func (s *WebhookService) markFailedOrRetry(ctx context.Context, d db.WebhookDelivery, status int, body, msg string) {
	nextAttempt := d.AttemptCount + 1
	statusParam := pgtype.Int4{Int32: int32(status), Valid: status > 0}
	bodyParam := pgtype.Text{String: body, Valid: body != ""}
	errParam := pgtype.Text{String: msg, Valid: msg != ""}
	if nextAttempt >= 5 {
		if _, err := s.Queries.MarkWebhookDeliveryFailed(ctx, db.MarkWebhookDeliveryFailedParams{
			ID:             d.ID,
			ResponseStatus: statusParam,
			ResponseBody:   bodyParam,
			Error:          errParam,
		}); err != nil {
			slog.Warn("mark webhook delivery failed failed", "delivery_id", util.UUIDToString(d.ID), "error", err)
		}
		return
	}
	backoff := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}
	delay := backoff[int(nextAttempt)-1]
	if _, err := s.Queries.MarkWebhookDeliveryRetrying(ctx, db.MarkWebhookDeliveryRetryingParams{
		ID:             d.ID,
		ResponseStatus: statusParam,
		ResponseBody:   bodyParam,
		Error:          errParam,
		NextAttemptAt:  pgtype.Timestamptz{Time: time.Now().Add(delay), Valid: true},
	}); err != nil {
		slog.Warn("mark webhook delivery retrying failed", "delivery_id", util.UUIDToString(d.ID), "error", err)
	}
}
