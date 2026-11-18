// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A job has no reader behind its answer, so it reads what every member may. A
// human's own gate decides for them, so nothing is added.
func TestOnlyTheSystemPrincipalIsNarrowedToTheWorkspaceAudience(t *testing.T) {
	t.Parallel()
	job := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	if got := ReaderlessAudience(job, "a"); got != AudienceWorkspaceOnly("a") {
		t.Errorf("the system principal reads %q, want the workspace audience", got)
	}
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test"})
	if got := ReaderlessAudience(human, "a"); got != "" {
		t.Errorf("a human is narrowed by %q, want nothing added", got)
	}
}

// A deal reader admits a private mail only when it is filed on that deal.
// It never admits a task, so a contact's private mail stays out of the deal.
func TestADealReaderAdmitsPrivateMailFiledOnTheDealOnly(t *testing.T) {
	t.Parallel()
	got := AudienceWorkspaceOrOnDeal("a", "d.id")
	for _, want := range []string{
		strings.TrimPrefix(AudienceWorkspaceOnly("a"), " AND "),
		PrivateAudienceSQL("a"),
		"a.kind <> 'task'",
		"private_on_deal.activity_id = a.id AND private_on_deal.deal_id = d.id",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AudienceWorkspaceOrOnDeal = %q, want it to carry %q", got, want)
		}
	}
	if strings.Contains(got, "%") {
		t.Errorf("AudienceWorkspaceOrOnDeal = %q holds a percent sign that a format string would read as a verb", got)
	}
}
