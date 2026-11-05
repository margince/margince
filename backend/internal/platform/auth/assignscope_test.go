// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// newRecordOwnerSQL swaps the status clause and nothing else. If the swap
// stopped matching (AssigneeEligibleSQL respelled its status clause), a new
// record would silently refuse an invited owner again; if it swapped more, the
// seat and scope halves of the rule would loosen with it.
func TestANewRecordMayNameAnInvitedOwner(t *testing.T) {
	p := principal.Principal{Type: principal.PrincipalHuman, ID: "user:test"}
	arg := func(any) int { return 1 }
	routing := AssigneeEligibleSQL(p, "u", arg)
	owner := newRecordOwnerSQL(p, "u", arg)

	if !strings.Contains(owner, workforce.ActivatableMemberSQL("u")) {
		t.Fatalf("a new record's owner check does not admit an invited seat:\n%s", owner)
	}
	if strings.Contains(routing, "invited") {
		t.Errorf("routing admits an invited seat; only a new record's owner may:\n%s", routing)
	}
	if strings.Replace(owner, workforce.ActivatableMemberSQL("u"), workforce.LiveMemberSQL("u"), 1) != routing {
		t.Errorf("the owner check differs from the routing check beyond the status clause:\nrouting: %s\nowner:   %s", routing, owner)
	}
}

// seatOutsideScopeTx answers that dest may own records and is outside the
// caller's scope: the scoped probe says no and the unscoped one yes.
type seatOutsideScopeTx struct {
	pgx.Tx
	probes int
}

func (tx *seatOutsideScopeTx) QueryRow(context.Context, string, ...any) pgx.Row {
	tx.probes++
	return answerRow(tx.probes > 1)
}

type answerRow bool

func (r answerRow) Scan(dest ...any) error {
	answer, ok := dest[0].(*bool)
	if !ok {
		return errors.New("answerRow: the probe scans a bool")
	}
	*answer = bool(r)
	return nil
}

// An eligible seat outside the caller's scope is refused as any other, and only
// a recorder learns that the scope refused it. Without one, nothing asks.
func TestAnAssigneeOutsideTheScopeIsNotedOnlyForARecorder(t *testing.T) {
	caller := human(principal.RowScopeOwn)
	shipped := &seatOutsideScopeTx{}
	var refused *AssigneeNotAllowedError
	if err := EnsureAssignee(principal.WithActor(context.Background(), caller), shipped, ids.NewV7()); !errors.As(err, &refused) {
		t.Fatalf("an assignee outside the scope was answered %v, want the assignee refusal", err)
	}
	if shipped.probes != 1 {
		t.Errorf("with no recorder the gate asked %d probes, want 1", shipped.probes)
	}

	rec := &checkRecorder{actor: caller.ID, checks: map[string]bool{}, refused: map[string]bool{}}
	ctx := context.WithValue(principal.WithActor(context.Background(), caller), checkRecorderKey{}, rec)
	if err := EnsureAssignee(ctx, &seatOutsideScopeTx{}, ids.NewV7()); !errors.As(err, &refused) {
		t.Fatalf("an assignee outside the scope was answered %v under a recorder, want the assignee refusal", err)
	}
	if got := slices.Sorted(maps.Keys(rec.refused)); !slices.Equal(got, []string{"row:assignee"}) {
		t.Errorf("recorded refusals %v, want [row:assignee]", got)
	}
}
