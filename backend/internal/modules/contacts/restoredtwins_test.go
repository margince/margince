// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Specs for filing the twins a restore creates. It carries the restore's own
// right and stops on a contact it cannot read. An import credits the provider
// whose episodes it applied.

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

// unreadableTx is a database whose every single-row read fails.
type unreadableTx struct {
	pgx.Tx
	err error
}

func (tx unreadableTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return failedRow{err: tx.err}
}

type failedRow struct{ err error }

func (r failedRow) Scan(...any) error { return r.err }

func TestFilingRestoredTwinsCarriesTheRestoreRight(t *testing.T) {
	user := ids.NewV7()
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{entityContact: {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	err := (&Store{}).FileRestoredEmployerTwinsTx(reader, nil, []ids.ContactID{ids.New[ids.ContactKind]()})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat that cannot restore = %v, want it refused: filing follows a restore", err)
	}
}

func TestFilingRestoredTwinsStopsOnAContactItCannotRead(t *testing.T) {
	ctx := seatRepairActor(map[string]principal.ObjectGrant{entityContact: {Read: true, Delete: true}})
	down := errors.New("connection refused")
	err := (&Store{}).FileRestoredEmployerTwinsTx(ctx, unreadableTx{err: down}, []ids.ContactID{ids.New[ids.ContactKind]()})
	if !errors.Is(err, down) || !strings.Contains(err.Error(), "reading the contact to re-check") {
		t.Errorf("a failed read = %v, want the database's error, named as the re-check read", err)
	}
}

func TestAnImportCreditsTheProviderWhoseEpisodesItApplied(t *testing.T) {
	episodes := []employmentEvidence{
		{key: "newer", provider: "provider-a"},
		{key: "selected", provider: "provider-b"},
	}
	key := "selected"
	if got := selectedEmploymentProvider(episodes, &key, ""); got != "provider-b" {
		t.Errorf("a keyed import credits %q, want provider-b, whose episode it applied", got)
	}
	if got := selectedEmploymentProvider(episodes, nil, ""); got != "provider-a" {
		t.Errorf("an unkeyed import credits %q, want the first episode's provider-a", got)
	}
	missing := "gone"
	if got := selectedEmploymentProvider(episodes, &missing, ""); got != "" {
		t.Errorf("an import that selects nothing credits %q, want nobody", got)
	}
}
