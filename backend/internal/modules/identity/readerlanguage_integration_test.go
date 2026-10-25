// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReaderLanguageIsTheCallersChoiceElseTheInstallationsBase(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)
	read := func(readCtx context.Context) string {
		var got string
		if err := svc.db.Tx(readCtx, func(tx pgx.Tx) error {
			got = string(ReaderLanguageForRecord(readCtx, tx))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(ctx); got != "en" {
		t.Fatalf("a member who never chose reads the base language en, got %q", got)
	}
	if _, err := svc.SaveMyLocale(ctx, "vi"); err != nil {
		t.Fatal(err)
	}
	if got := read(ctx); got != "vi" {
		t.Fatalf("a member who chose vi reads vi, got %q", got)
	}
	system := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	if got := read(system); got != "en" {
		t.Fatalf("a system caller has no display language and reads the base language, got %q", got)
	}
}
