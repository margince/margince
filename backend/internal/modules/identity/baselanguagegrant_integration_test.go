// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A caller who may not read the installation's settings writes shared prose in
// English. Whoever calls, choosing the language requires no grant of the call.
func TestTheBaseLanguageIsNoRequirementOfTheCall(t *testing.T) {
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:no-settings", PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
	})
	ctx, recorded := auth.RecordChecksForTest(ctx)
	if lang := BaseLanguageForRecord(ctx, nil); lang != textlang.English {
		t.Errorf("a caller without the grant wrote in %s, want English", lang)
	}
	if lang := BaseLanguageForPrompt(ctx, nil); lang != string(textlang.English) {
		t.Errorf("a caller without the grant was prompted in %s, want English", lang)
	}
	if checks := recorded().Checks; len(checks) != 0 {
		t.Errorf("choosing the language required %v of the call", checks)
	}

	reader := principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:settings-reader", PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{installationSettingsObject: {Read: true}}},
	}
	readerCtx, read := auth.RecordChecksForTest(principal.WithActor(ctx, reader))
	BaseLanguageForRecord(readerCtx, failingTx{})
	if checks := read().Checks; len(checks) != 0 {
		t.Errorf("a caller holding the grant had %v recorded as needed", checks)
	}
}

// failingTx answers every statement with an error, so the read degrades.
type failingTx struct{ pgx.Tx }

func (failingTx) QueryRow(context.Context, string, ...any) pgx.Row { return failingRow{} }

type failingRow struct{}

func (failingRow) Scan(...any) error { return errors.New("failingRow: no database here") }
