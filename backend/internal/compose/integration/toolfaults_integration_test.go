// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A wrong tool argument or a missing passport scope, against real stores and
// the real HTTP stack. The caller is told what to fix, not to retry.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Every record type with an id, flag or number filter. Partner takes only
// words, so it has no operand that can be malformed.
func TestAMistypedListFilterIsTheCallersMistakeOnEveryRecordType(t *testing.T) {
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	for _, call := range []string{
		`{"record_type":"company","filters":{"owner_id":"not-a-uuid"}}`,
		`{"record_type":"contact","filters":{"tag_id":"not-a-uuid"}}`,
		`{"record_type":"deal","filters":{"stalled":"maybe"}}`,
		`{"record_type":"lead","filters":{"min_score":"high"}}`,
		`{"record_type":"project","filters":{"company_id":"not-a-uuid"}}`,
	} {
		_, err := registry.Invoke(ctx, "list_records", json.RawMessage(call))

		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %v, want a 422 the caller can fix rather than an internal fault", call, err)
		}
	}
}

func TestAPassportWithoutReadIsToldWhichPermissionIsMissing(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Read Scope", "readscope@fable.test", "Admin")
	draftOnly := apptest.PassportBearer(t, e, "draft-only agent", "draft")

	var refusal capRefusal
	status := e.Call(t, "GET", "/v1/companies?limit=1", nil, draftOnly, &refusal)

	if status != http.StatusForbidden || refusal.Code != scopeRefusalCode {
		t.Fatalf("a read on a draft-only passport → %d %q, want 403 %s", status, refusal.Code, scopeRefusalCode)
	}
	if !strings.Contains(refusal.Detail, `"read"`) {
		t.Errorf("the refusal does not name the missing permission: %q", refusal.Detail)
	}
}
