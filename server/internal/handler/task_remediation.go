package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
)

type remediateFailureRequest struct {
	FailedTaskID   string  `json:"failed_task_id"`
	Action         string  `json:"action"`
	TargetAgentID  *string `json:"target_agent_id"`
	RemediationKey string  `json:"remediation_key"`
	Reason         string  `json:"reason"`
}

type remediateFailureResponse struct {
	ID            string  `json:"id"`
	Action        string  `json:"action"`
	IssueID       string  `json:"issue_id"`
	CreatedTaskID *string `json:"created_task_id"`
	Idempotent    bool    `json:"idempotent"`
}

func (h *Handler) RemediateIssueFailure(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}

	var req remediateFailureRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeRemediationError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	failedTaskID, ok := parseUUIDOrBadRequest(w, req.FailedTaskID, "failed_task_id")
	if !ok {
		return
	}
	action := strings.ReplaceAll(strings.TrimSpace(req.Action), "-", "_")
	var targetAgentID pgtype.UUID
	if req.TargetAgentID != nil {
		targetAgentID, ok = parseUUIDOrBadRequest(w, *req.TargetAgentID, "target_agent_id")
		if !ok {
			return
		}
	}

	result, err := h.TaskService.RemediateTaskFailure(r.Context(), service.RemediateTaskFailureParams{
		WorkspaceID: issue.WorkspaceID, IssueID: issue.ID, FailedTaskID: failedTaskID,
		Action: action, TargetAgentID: targetAgentID, RemediationKey: req.RemediationKey,
		Reason: req.Reason, CreatedBy: userUUID, Automated: true,
	})
	if err != nil {
		var remediationErr *service.RemediationError
		if errors.As(err, &remediationErr) {
			status := remediationErrorStatus(remediationErr.Code)
			writeRemediationError(w, status, remediationErr.Code, remediationErr.Message)
			return
		}
		writeRemediationError(w, http.StatusInternalServerError, "remediation_failed", "failed to remediate task failure")
		return
	}

	var createdTaskID *string
	if result.Remediation.CreatedTaskID.Valid {
		id := uuidToString(result.Remediation.CreatedTaskID)
		createdTaskID = &id
	}
	status := http.StatusAccepted
	if result.Idempotent {
		status = http.StatusOK
	}
	writeJSON(w, status, remediateFailureResponse{
		ID: uuidToString(result.Remediation.ID), Action: strings.ReplaceAll(result.Remediation.Action, "_", "-"),
		IssueID: uuidToString(result.Remediation.IssueID), CreatedTaskID: createdTaskID, Idempotent: result.Idempotent,
	})
}

func remediationErrorStatus(code string) int {
	switch code {
	case "issue_not_found", "failed_task_not_found", "target_agent_not_found":
		return http.StatusNotFound
	case "active_task_exists", "remediation_budget_exhausted", "remediation_key_conflict", "task_not_failed", "failed_task_already_remediated":
		return http.StatusConflict
	case "unsafe_remediation", "unsafe_reason", "same_target_agent", "target_agent_archived", "target_agent_offline", "target_runtime_missing", "target_runtime_offline":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

func writeRemediationError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
