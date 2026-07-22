package agent

import (
	"encoding/json"
	"testing"
)

func TestSelectACPPermissionOption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		options   string
		wantID    string
		wantGrant bool
		wantOK    bool
	}{
		{name: "Hermes edit allow once", options: `[{"optionId":"allow_once","kind":"allow_once"},{"optionId":"deny","kind":"reject_once"}]`, wantID: "allow_once", wantGrant: true, wantOK: true},
		{name: "session grant preferred", options: `[{"optionId":"single","kind":"allow_once"},{"optionId":"allow_session","kind":"allow_always"}]`, wantID: "allow_session", wantGrant: true, wantOK: true},
		{name: "permanent grant rejected in favor of single reject", options: `[{"optionId":"allow_always","kind":"allow_always"},{"optionId":"deny","kind":"reject_once"}]`, wantID: "deny", wantOK: true},
		{name: "permanent only fails closed", options: `[{"optionId":"allow_always","kind":"allow_always"}]`},
		{name: "empty fails closed", options: `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := json.RawMessage(`{"options":` + tt.options + `}`)
			id, grant, ok := selectACPPermissionOption(params)
			if id != tt.wantID || grant != tt.wantGrant || ok != tt.wantOK {
				t.Fatalf("got (%q,%v,%v), want (%q,%v,%v)", id, grant, ok, tt.wantID, tt.wantGrant, tt.wantOK)
			}
		})
	}
}
