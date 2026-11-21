// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheReportNamesLeftRecordsOnlyToAReviewer(t *testing.T) {
	left := []datatask.Progress{{TaskID: "core/1_fix", Step: "a", Status: datatask.Done, Action: &datatask.ActionNeeded{
		Reason: "privacy_reviewer_missing", Count: 1, Records: []datatask.RecordRef{{Type: "approval_history", ID: ids.NewV7()}},
	}}}
	operator := principal.Principal{Type: principal.PrincipalHuman, ID: "user:operator", Permissions: principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"data_task": {Read: true}}, RowScope: principal.RowScopeAll,
	}}
	reviewer := operator
	reviewer.Permissions.Objects = map[string]principal.ObjectGrant{"data_task": {Read: true}, "audit_log": {Read: true}}
	for name, c := range map[string]struct {
		who  principal.Principal
		want int
	}{"operator": {operator, 0}, "reviewer": {reviewer, 1}} {
		got := dataTaskProgressForReader(principal.WithActor(t.Context(), c.who), left)[0].ActionNeeded
		if got == nil || got.Count != 1 || len(got.Records) != c.want {
			t.Errorf("%s got %+v, want %d records", name, got, c.want)
		}
	}
}
