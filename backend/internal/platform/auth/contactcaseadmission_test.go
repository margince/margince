// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refusingTx answers every EXISTS probe "no" and keeps what it was asked.
type refusingTx struct {
	pgx.Tx
	asked []string
}

func (tx *refusingTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	tx.asked = append(tx.asked, sql)
	return falseRow{}
}

type falseRow struct{}

func (falseRow) Scan(dest ...any) error {
	if answer, ok := dest[0].(*bool); ok {
		*answer = false
		return nil
	}
	return errors.New("falseRow: unexpected scan target")
}

func caseAdmitted(t *testing.T, members, grantees []ids.UUID) context.Context {
	t.Helper()
	ctx := principal.WithActor(context.Background(), human(principal.RowScopeOwn))
	ctx, err := WithContactCaseAdmission(ctx, members, grantees)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestACaseAdmissionWritesOnlyTheSelectedContacts(t *testing.T) {
	member, other, owner := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ctx := caseAdmitted(t, []ids.UUID{member, ids.NewV7()}, []ids.UUID{owner})

	tx := &refusingTx{}
	if err := ensureWriteAuthority(ctx, tx, "contact", member); err != nil {
		t.Fatalf("a selected contact was refused: %v", err)
	}
	if len(tx.asked) != 0 {
		t.Fatalf("a selected contact still asked the write predicate: %v", tx.asked)
	}
	for _, probe := range []struct {
		table string
		id    ids.UUID
	}{{"contact", other}, {"company", member}, {"lead", member}} {
		if err := ensureWriteAuthority(ctx, &refusingTx{}, probe.table, probe.id); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s %s outside the admission = %v, want the ordinary refusal", probe.table, probe.id, err)
		}
	}
}

func TestACaseAdmissionAssignsOnlyToTheNamedOwners(t *testing.T) {
	owner, stranger := ids.NewV7(), ids.NewV7()
	ctx := caseAdmitted(t, []ids.UUID{ids.NewV7(), ids.NewV7()}, []ids.UUID{owner})

	named := &refusingTx{}
	if err := EnsureAssignee(ctx, named, owner); err == nil {
		t.Fatal("the refusing database admitted the named owner, so the seat half was not asked")
	}
	if strings.Contains(named.asked[0], "u.owner_id") {
		t.Errorf("a named owner was still judged by the caller's write scope:\n%s", named.asked[0])
	}
	if !strings.Contains(named.asked[0], "seat_type <> 'read'") {
		t.Errorf("a named owner skipped the seat half:\n%s", named.asked[0])
	}
	unnamed := &refusingTx{}
	if err := EnsureAssignee(ctx, unnamed, stranger); err == nil {
		t.Fatal("an owner outside the admission was admitted")
	}
	if !strings.Contains(unnamed.asked[0], "u.owner_id") {
		t.Errorf("an owner outside the admission lost the scope half:\n%s", unnamed.asked[0])
	}
}

func TestACaseAdmissionNamesWhatItAdmits(t *testing.T) {
	if _, err := WithContactCaseAdmission(context.Background(), []ids.UUID{ids.NewV7()}, []ids.UUID{ids.NewV7()}); err == nil {
		t.Error("one contact is not a case")
	}
	if _, err := WithContactCaseAdmission(context.Background(), []ids.UUID{ids.NewV7(), ids.NewV7()}, nil); err == nil {
		t.Error("an admission with no owner hands the survivor to nobody")
	}
}
