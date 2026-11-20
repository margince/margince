// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The attach direction as the clause SPELLS it: what a reader sees, narrowed
// to what the caller may change.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func attachClause(t *testing.T, p principal.Principal, table string) string {
	t.Helper()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	clause, err := AttachClauseFor(principal.WithActor(context.Background(), p), table, "t", arg)
	if err != nil {
		t.Fatalf("AttachClauseFor(%s): %v", table, err)
	}
	return clause
}

// Every seat reads every deal, so visibility alone renders no owner arm. The
// attach clause must still name the owner and admit only a `write` grant.
func TestAnAttachNeedsTheOwnerArmOrAWriteGrant(t *testing.T) {
	got := attachClause(t, human(principal.RowScopeTeam), tableDeal)
	if !strings.Contains(got, "t.owner_id") {
		t.Errorf("the attach clause does not ask who owns the record:\n%s", got)
	}
	if !strings.Contains(got, "rg.access = 'write'") {
		t.Errorf("the attach clause admits a read-only share:\n%s", got)
	}
}

// Capture privacy stays: the write arm does not carry it, so the visibility
// half must still be there for an owner-private contact.
func TestAnAttachStillFacesCapturePrivacy(t *testing.T) {
	got := attachClause(t, human(principal.RowScopeTeam), tableContact)
	if !strings.Contains(got, "t.visibility") || !strings.Contains(got, "team_membership") {
		t.Errorf("the attach clause lost capture privacy or the team's write arm:\n%s", got)
	}
}

// An all-scope reader holds write authority over every row, so the write
// half renders nothing. A capture-private contact they see only through a
// share must still need that share at `write`.
func TestAnAllScopeAttachToAPrivateContactNeedsAWriteShare(t *testing.T) {
	got := attachClause(t, human(principal.RowScopeAll), tableContact)
	if !strings.Contains(got, "t.visibility") || !strings.Contains(got, "rg.access = 'write'") {
		t.Errorf("an all-scope attach admits a capture-private contact through a read share:\n%s", got)
	}
}

// The system principal changes every row, so it is not narrowed at all.
func TestTheSystemPrincipalAttachesWithoutNarrowing(t *testing.T) {
	system := principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"}
	if got := attachClause(t, system, tableContact); got != "" {
		t.Errorf("the system principal's attach clause narrows: %s", got)
	}
}

// A merge admission lets ensureWriteAuthority change its rows, so the clause
// a list renders admits them too.
func TestAnAttachClauseAdmitsAMergesAdmittedRows(t *testing.T) {
	left, right := ids.NewV7(), ids.NewV7()
	ctx, err := WithDuplicatePairAdmission(principal.WithActor(context.Background(), human(principal.RowScopeOwn)), tableContact, left, right)
	if err != nil {
		t.Fatal(err)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	clause, err := AttachClauseFor(ctx, tableContact, "t", arg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "t.id = ANY(") || !slices.ContainsFunc(args, func(v any) bool {
		admitted, ok := v.([]ids.UUID)
		return ok && slices.Equal(admitted, []ids.UUID{left, right})
	}) {
		t.Errorf("the attach clause leaves out the admitted rows:\n%s", clause)
	}
}

func TestAnAttachRefusalNamesTheRemedyAndAnswers403(t *testing.T) {
	err := error(&AttachAuthorityError{Kind: tableDeal})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the refusal does not unwrap to ErrPermissionDenied: %v", err)
	}
	if _, message := (&AttachAuthorityError{Kind: tableDeal}).MessageFault(); !strings.Contains(message, "this deal") {
		t.Errorf("the refusal does not name the record kind: %q", message)
	}
}
