// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInvoiceReferenceUsesFinanceAndCompanyAuthority(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	summary := e.summaryAtEpoch(t, e.company)
	if summary.RecentInvoices == nil || len(*summary.RecentInvoices) == 0 {
		t.Fatal("no mirrored invoice")
	}
	invoice := (*summary.RecentInvoices)[0]
	company, number, err := e.store.InvoiceReference(e.ctx, ids.UUID(invoice.Id))
	if err != nil {
		t.Fatal(err)
	}
	if company != e.company || invoice.Number == nil || number != *invoice.Number {
		t.Fatalf("reference differs from finance read: %v %q", company, number)
	}
	owner := ids.NewV7()
	args := []any{owner, owner.String() + "@example.test"}
	e.ownerExec(t, fmt.Sprintf(`INSERT INTO app_user (id,email,display_name) VALUES ($%d,$%d,'Invoice owner')`, len(args)-1, len(args)), args...)
	args = []any{owner, "human:" + owner.String(), e.company}
	e.ownerExec(t, fmt.Sprintf(`UPDATE company SET owner_id=$%d, captured_by=$%d, visibility='owner' WHERE id=$%d`, len(args)-2, len(args)-1, len(args)), args...)
	for _, readFinance := range []bool{false, true} {
		user := ids.NewV7()
		ctx := principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: map[string]principal.ObjectGrant{"finance": {Read: readFinance}, "company": {Read: true}}}})
		company, number, err := e.store.InvoiceReference(ctx, ids.UUID(invoice.Id))
		want := apperrors.ErrPermissionDenied
		if readFinance {
			want = apperrors.ErrNotFound
		}
		if !errors.Is(err, want) || company.UUID != ids.Nil || number != "" {
			t.Fatalf("readFinance=%v disclosed reference: %v %q %v", readFinance, company, number, err)
		}
	}
	if _, _, err := e.store.InvoiceReference(e.ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("missing invoice: %v", err)
	}
}
