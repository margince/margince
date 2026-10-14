// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBindReturnsTheErrorOfAWriteThatCannotOpen(t *testing.T) {
	w := &csvWriters{links: newTestLinkResolver(t), object: migration.ObjectCompany}
	err := w.bind(context.Background(), migration.Row{ExternalID: "CO-1"}, ids.NewV7())
	if !errors.Is(err, database.ErrNoWorkspace) {
		t.Fatalf("bind = %v, want the transaction's refusal returned", err)
	}
}

func TestARefIsFreeToTheCompanyItAlreadyStandsForAndToAnUndoneRun(t *testing.T) {
	mine, other := ids.NewV7(), ids.NewV7()
	w := &csvWriters{bindings: map[string]migration.Binding{
		"same":   {NativeID: mine, RunStatus: migration.StatusComplete, Origin: migration.OriginCreated},
		"taken":  {NativeID: other, RunStatus: migration.StatusComplete, Origin: migration.OriginBound},
		"undone": {NativeID: other, RunStatus: migration.StatusUndone, Origin: migration.OriginBound},
	}}
	ctx := context.Background()
	if got := w.refTakenBy(ctx, "same", mine); got != "" {
		t.Errorf("a ref bound to the named company again = %q, want free", got)
	}
	if got := w.refTakenBy(ctx, "undone", mine); got != "" {
		t.Errorf("a ref of an undone run = %q, want free", got)
	}
	if got := w.refTakenBy(ctx, "unseen", mine); got != "" {
		t.Errorf("an unmapped ref = %q, want free", got)
	}
	if got := w.refTakenBy(ctx, "", mine); got != "" {
		t.Errorf("a row without a ref = %q, want free", got)
	}
	if got := w.refTakenBy(ctx, "taken", mine); got == "" {
		t.Error("a ref mapped to another company is free, want a reason")
	}
}

func TestAnUndoneRunsBoundRefDoesNotResolve(t *testing.T) {
	w := &csvWriters{
		object: migration.ObjectCompany, nativeIDs: map[string]ids.UUID{},
		bindings: map[string]migration.Binding{
			"c1": {NativeID: ids.NewV7(), RunStatus: migration.StatusUndone, Origin: migration.OriginBound},
		},
	}
	if _, found, err := w.lookup(context.Background(), migration.ObjectCompany, "c1"); err != nil || found {
		t.Fatalf("lookup = found %v, err %v; want the key free", found, err)
	}
}
