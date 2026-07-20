package handler

import (
	"net/http"
	"testing"
)

func TestRemediationErrorStatus(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		"issue_not_found":              http.StatusNotFound,
		"failed_task_not_found":        http.StatusNotFound,
		"target_agent_not_found":       http.StatusNotFound,
		"active_task_exists":           http.StatusConflict,
		"remediation_budget_exhausted": http.StatusConflict,
		"remediation_key_conflict":     http.StatusConflict,
		"task_not_failed":              http.StatusConflict,
		"unsafe_remediation":           http.StatusUnprocessableEntity,
		"unsafe_reason":                http.StatusUnprocessableEntity,
		"same_target_agent":            http.StatusUnprocessableEntity,
		"target_agent_archived":        http.StatusUnprocessableEntity,
		"target_agent_offline":         http.StatusUnprocessableEntity,
		"target_runtime_missing":       http.StatusUnprocessableEntity,
		"target_runtime_offline":       http.StatusUnprocessableEntity,
		"invalid_action":               http.StatusBadRequest,
	}
	for code, want := range tests {
		if got := remediationErrorStatus(code); got != want {
			t.Errorf("remediationErrorStatus(%q) = %d, want %d", code, got, want)
		}
	}
}
