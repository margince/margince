// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// log_activity's schema is closed, so a colleague list it does not declare is
// one a schema-driven client can never send.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLogActivityDeclaresTheColleaguesWhoAttended(t *testing.T) {
	var schema struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(logActivity{}.Spec().InputSchema, &schema); err != nil {
		t.Fatalf("decoding the log_activity schema: %v", err)
	}
	field, ok := schema.Properties["attendee_user_ids"]
	if !ok {
		t.Fatal("log_activity does not declare attendee_user_ids")
	}
	got := string(field["type"]) + " " + string(field["maxItems"]) + " " + string(field["items"])
	if want := `"array" 25 {"type":"string","format":"uuid"}`; got != want {
		t.Fatalf("attendee_user_ids = %s, want %s", got, want)
	}
}

// The tool forwards its arguments whole, so the colleagues reach the same
// strict decode against CreateActivityRequest that the REST create runs.
func TestLogActivityCarriesTheColleaguesToTheContractBody(t *testing.T) {
	s := idProbeDispatcher(t)
	ctx := scopedAgentCtx(principal.ScopeWrite)
	call := `{"name":"log_activity","arguments":{"kind":"meeting","subject":"Review",` +
		`"attendee_user_ids":["` + ids.NewV7().String() + `"]}}`

	answer := errorText(t, callMap(ctx, t, s, call))
	// The probe seam answers with errSeamReached only after the decode passed.
	if !strings.Contains(answer, internalFaultAdvice) {
		t.Fatalf("log_activity refused attendee_user_ids before the seam: %q", answer)
	}
}
