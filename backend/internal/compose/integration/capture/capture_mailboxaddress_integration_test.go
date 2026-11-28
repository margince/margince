// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A seat holds the message through the mailbox it arrived in, and the email
// details name that address, not the seat's login.
func TestEmailDetailsNameTheMailboxAMessageArrivedIn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	const mailbox = "colleague+second@myco.example"
	seedCaptureRoleFor(t, e, e.Rep3)
	syncSecond := mailboxFor(t, e, e.Rep3, mailbox, compose.CaptureConfig{})

	syncSecond(t, email("anna@acme.example", "Anna Acme", mailbox, "second-box@acme.example", ""))

	var login string
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id = $1`, e.Rep3).Scan(&login)
	}); err != nil {
		t.Fatalf("reading the seat's login: %v", err)
	}
	id := oneActivityID(t, e)
	got, err := activities.NewStore(e.DB()).GetEmailPresentation(readerCtx(e, e.Rep3), ids.From[ids.ActivityKind](id), nil)
	if err != nil {
		t.Fatalf("reading the email details: %v", err)
	}
	var seen []string
	for _, p := range got.To {
		seen = append(seen, strings.ToLower(p.Address))
	}
	if len(seen) != 1 || seen[0] != mailbox {
		t.Fatalf("To = %v, want only the mailbox %q the message arrived in (seat login %q)", seen, mailbox, login)
	}
}
