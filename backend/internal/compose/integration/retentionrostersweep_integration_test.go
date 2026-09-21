// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The nightly retention sweep against a chat roster, where it differs from the
// Art. 17 eraser in the one way that matters: NOBODY ASKED FOR IT.
//
// The eraser is allowed to reach a subject by every identifier they hold,
// archived bindings included, because refuseRivalIdentifierHolders has already
// refused the whole request if a live contact holds one of the same ones. This
// sweep has no such refusal — there is no request to refuse — so it may only
// match identifiers that still name the subject and nobody else.

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedRosterOn records one chat message with one roster party named by account.
func seedRosterOn(t *testing.T, account string) ids.UUID {
	t.Helper()
	activity := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, channel_provider)
		VALUES ($1, 'message', 'the group', now(), 'inbound', 'telegram', 'connector:telegram', 'telegram')`)
	return SeedIDRow(t, OwnerConn(t), `INSERT INTO activity_participant (id, activity_id, channel_user_id, role)
		VALUES ($1, '`+activity.String()+`', '`+account+`', 'attendee')`)
}

func rowStillThere(t *testing.T, row ids.UUID) bool {
	t.Helper()
	var found bool
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM activity_participant WHERE id = $1)`, row).Scan(&found); err != nil {
		t.Fatalf("reading the roster row back: %v", err)
	}
	return found
}

// THE OVER-DELETION THE SWEEP MUST NOT COMMIT. Archiving a contact archives
// their channel bindings, so the same customer writing in again resolves to a
// SECOND contact who now holds that account live. When a clock later reaches the
// archived record, matching its archived binding would delete the live contact's
// rows — on a sweep that named only the archived one.
func TestTheSweepDoesNotReachARosterRowNowHeldByALiveContact(t *testing.T) {
	e := Setup(t)
	SeedRetentionPolicies(t, e)
	owner := OwnerConn(t)

	// The contact the sweep will reach: old enough for the unattached-contact
	// selector, still live so the selector admits them, and holding a RETIRED
	// binding on the account — the state left behind when a rebind moved it.
	stale := SeedIDRow(t, owner, `INSERT INTO contact (id, full_name, source, captured_by, created_at)
		VALUES ($1, 'Old Contact', 'manual', 'human:x', now() - interval '800 days')`)
	SeedIDRow(t, owner, `INSERT INTO contact_channel_identity (id, contact_id, provider, channel_user_id, source, captured_by, archived_at)
		VALUES ($1, '`+stale.String()+`', 'telegram', 'acct-shared', 'capture', 'connector:telegram', now())`)

	// The same human, captured again, live and holding the account now.
	live := e.SeedContact(t, "The Same Human, Again", &e.Rep1)
	SeedIDRow(t, owner, `INSERT INTO contact_channel_identity (id, contact_id, provider, channel_user_id, source, captured_by)
		VALUES ($1, '`+live.String()+`', 'telegram', 'acct-shared', 'capture', 'connector:telegram')`)

	theirs := seedRosterOn(t, "acct-shared")

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("retention pass: %v", err)
	}

	if !rowStillThere(t, theirs) {
		t.Fatal("the sweep deleted a roster row belonging to a LIVE contact, because an archived record once held the same account — the Art. 17 path refuses that case outright and this one has nobody to refuse")
	}
}
