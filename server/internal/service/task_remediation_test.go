package service

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRemediationActionSafeMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action, reason string
		want           bool
	}{
		{RemediationActionRerun, "timeout", true},
		{RemediationActionRerun, "runtime_offline", true},
		{RemediationActionRerun, "runtime_recovery", true},
		{RemediationActionRerun, "rate_limit", false},
		{RemediationActionRerun, "agent_error", false},
		{RemediationActionReassignRerun, "rate_limit", true},
		{RemediationActionReassignRerun, "context_limit", true},
		{RemediationActionReassignRerun, "model_limit", true},
		{RemediationActionReassignRerun, "model_access", true},
		{RemediationActionReassignRerun, "model_not_found", true},
		{RemediationActionReassignRerun, "timeout", false},
		{RemediationActionBlock, "agent_error", true},
		{"delete", "timeout", false},
	}
	for _, tt := range tests {
		t.Run(tt.action+"/"+tt.reason, func(t *testing.T) {
			if got := remediationActionSafe(tt.action, tt.reason); got != tt.want {
				t.Fatalf("remediationActionSafe(%q, %q) = %v, want %v", tt.action, tt.reason, got, tt.want)
			}
		})
	}
}

func TestValidateRemediationInput(t *testing.T) {
	t.Parallel()
	target := pgtype.UUID{Valid: true}
	valid := RemediateTaskFailureParams{Action: RemediationActionRerun, RemediationKey: "task:timeout", Reason: "retry timeout"}
	tests := []struct {
		name string
		edit func(*RemediateTaskFailureParams)
		code string
	}{
		{"empty key", func(p *RemediateTaskFailureParams) { p.RemediationKey = " " }, "invalid_remediation_key"},
		{"long key", func(p *RemediateTaskFailureParams) { p.RemediationKey = strings.Repeat("界", 201) }, "invalid_remediation_key"},
		{"empty reason", func(p *RemediateTaskFailureParams) { p.Reason = "" }, "invalid_reason"},
		{"long reason", func(p *RemediateTaskFailureParams) { p.Reason = strings.Repeat("界", 501) }, "invalid_reason"},
		{"control reason", func(p *RemediateTaskFailureParams) { p.Reason = "line one\nline two" }, "unsafe_reason"},
		{"bad action", func(p *RemediateTaskFailureParams) { p.Action = "retry" }, "invalid_action"},
		{"missing target", func(p *RemediateTaskFailureParams) { p.Action = RemediationActionReassignRerun }, "target_agent_required"},
		{"unexpected target", func(p *RemediateTaskFailureParams) { p.TargetAgentID = target }, "unexpected_target_agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.edit(&p)
			err := validateRemediationInput(p)
			remediationErr, ok := err.(*RemediationError)
			if !ok || remediationErr.Code != tt.code {
				t.Fatalf("error = %#v, want RemediationError code %q", err, tt.code)
			}
		})
	}
	if err := validateRemediationInput(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}
