// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The classification is asked with the staged call, so a credential's release
// follows what THIS call does; an unattended run, or an engine built without the
// classification, releases nothing.
func TestAStagedCallIsReleasableByTheCallerOnlyWhenUndoableAndAttended(t *testing.T) {
	sawChange := json.RawMessage(`{"entity_type":"company"}`)
	undoableCompanyOnly := func(_, _ string, change json.RawMessage) bool { return string(change) == string(sawChange) }

	attended := context.Background()
	unattended := principal.WithAgentRunID(attended, ids.NewV7())

	for _, tc := range []struct {
		name   string
		svc    *Service
		ctx    context.Context
		change json.RawMessage
		want   bool
	}{
		{"an undoable call, attended", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended, sawChange, true},
		{"a call that is not undoable", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended, json.RawMessage(`{"entity_type":"project"}`), false},
		{"an unattended run", NewService(nil).WithUndoableRelease(undoableCompanyOnly), unattended, sawChange, false},
		{"no classification installed", NewService(nil), attended, sawChange, false},
	} {
		if got := tc.svc.ReleasableByCaller(tc.ctx, "relink_activities", "company", tc.change); got != tc.want {
			t.Errorf("%s: ReleasableByCaller = %v, want %v", tc.name, got, tc.want)
		}
	}
}
