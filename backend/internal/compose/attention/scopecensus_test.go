// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestEverySourceRespectsTheSelectedScope(t *testing.T) {
	repOnTeamA := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	repOnTeamB := ids.NewV7()
	sources := everyReachSource(t)
	shared := 0
	for _, source := range sources {
		if !sourceAnswersForTheActorOnly[source] {
			shared++
		}
	}
	if shared == 0 || shared == len(sources) {
		t.Fatal("source census must cover shared and personal work")
	}
	for _, source := range sources {
		for _, tc := range []struct {
			name    string
			tier    principal.RowScope
			scope   string
			owner   ids.UUID
			members roster
			want    []string
		}{
			{"rep mine", principal.RowScopeOwn, scopeMine, ids.Nil, nil, []string{"a"}},
			{"lead mine", principal.RowScopeTeam, scopeMine, ids.Nil, nil, []string{"a"}},
			{"management mine", principal.RowScopeAll, scopeMine, ids.Nil, nil, []string{"a"}},
			{"team A lead", principal.RowScopeTeam, scopeTeam, ids.Nil, roster{{UserID: repOnTeamA}}, []string{"a", "unassigned"}},
			{"team B lead", principal.RowScopeTeam, scopeTeam, ids.Nil, roster{{UserID: repOnTeamB}}, []string{"b", "unassigned"}},
			{"management all", principal.RowScopeAll, scopeAll, ids.Nil, nil, []string{"a", "b", "unassigned"}},
			{"rep unassigned", principal.RowScopeOwn, scopeUnassigned, ids.Nil, nil, []string{"unassigned"}},
			{"lead named owner", principal.RowScopeTeam, scopeTeam, repOnTeamB, nil, []string{"b"}},
		} {
			t.Run(string(source)+"/"+tc.name, func(t *testing.T) {
				input := []ranked{
					{item: crmcontracts.WorklistItem{Id: "a", Source: source}, ownerRef: ownedBy(repOnTeamA)},
					{item: crmcontracts.WorklistItem{Id: "b", Source: source}, ownerRef: ownedBy(repOnTeamB)},
					{item: crmcontracts.WorklistItem{Id: "unassigned", Source: source}, ownerRef: unassigned()},
				}
				input, want := admittedScopeFixture(source, input, tc.scope, tc.owner, tc.want)
				svc := &Service{teammates: tc.members}
				got, note := svc.narrowToScope(readerAt(tc.tier), input, tc.scope, tc.owner)
				if note.failed || note.truncated {
					t.Fatalf("unexpected incomplete scope: %+v", note)
				}
				actual := make([]string, 0, len(got))
				for _, row := range got {
					actual = append(actual, row.item.Id)
				}
				if !slices.Equal(actual, want) {
					t.Fatalf("got %v, want %v", actual, want)
				}
			})
		}
	}
}

// Store-owned scopes must supply admitted rows; this display cut cannot re-authorize
// a meeting participant or another reader's mailbox. Real store tests cover admission.
func admittedScopeFixture(source crmcontracts.WorklistItemSource, rows []ranked, scope string, owner ids.UUID, want []string) ([]ranked, []string) {
	if sourceAnswersForTheActorOnly[source] {
		if scope == scopeUnassigned || !owner.IsZero() {
			return nil, nil
		}
		return []ranked{{item: crmcontracts.WorklistItem{Id: "personal", Source: source}, ownerRef: ownedByWhoeverIsReading()}}, []string{"personal"}
	}
	switch source {
	case sourceTask, sourceNoticeCase, sourceMeeting, sourceMeetingOutcome:
		if !owner.IsZero() {
			return rows[1:2], want
		}
		if scope == scopeMine {
			return rows[:1], want
		}
	case sourceBriefItem:
		if !owner.IsZero() {
			return rows, nil
		}
		if scope == scopeMine {
			return rows[:1], want
		}
	}
	return rows, want
}
