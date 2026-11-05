// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestListInputChecksPassesItsScopeToTheRead(t *testing.T) {
	var asked InputChecksArgs
	tool := listInputChecks{read: func(_ context.Context, args InputChecksArgs) (json.RawMessage, error) {
		asked = args
		return json.RawMessage(`{"data":[]}`), nil
	}}

	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"scope_kind":"team","scope_id":"0190a1b2-0000-7000-8000-000000000001"}`)); err != nil {
		t.Fatal(err)
	}
	if asked.ScopeKind != "team" || asked.ScopeID != "0190a1b2-0000-7000-8000-000000000001" {
		t.Errorf("read got %+v, want the team scope", asked)
	}

	asked = InputChecksArgs{ScopeKind: "stale"}
	if _, err := tool.Handle(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if asked != (InputChecksArgs{}) {
		t.Errorf("a call with no arguments read %+v, want no scope", asked)
	}

	var bad *BadArgsError
	if _, err := tool.Handle(context.Background(), json.RawMessage(`{"owner":"x"}`)); !errors.As(err, &bad) {
		t.Errorf("an unknown argument: err = %v, want it refused by name", err)
	}
}
