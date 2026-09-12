// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A grant whose expiry has already passed is refused, not stored.
//
// Both predicates that read the column test `expires_at IS NULL OR expires_at >
// now()`, so a back-dated grant is inert from the instant it is written. Inert
// is not harmless, because the ROW IS THERE: the share list names somebody who
// in fact has no access, and a human scanning who can see a record is told the
// opposite of the truth. Nothing sweeps the table either, so the mistake
// accumulates permanently (#2139).
//
// No database and no actor, for the reason requiredids_test.go gives: the guard
// is a request-shape refusal and runs before any authority check or query, so a
// store over a nil pool reaches it. That is also the assertion — a check that
// had drifted below the auth gate would fail here rather than pass quietly.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// grantNoon is a fixed clock, so "already past" is a property of the input rather
// than of how long the test took to run.
var grantNoon = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func grantAt(t *testing.T, expiresAt *time.Time) error {
	t.Helper()
	svc := NewService(nil)
	svc.now = func() time.Time { return grantNoon }
	_, err := svc.CreateRecordGrant(context.Background(), CreateGrantInput{
		RecordType: "contact", SubjectType: "user", Access: "read",
		RecordID: ids.NewV7(), SubjectID: ids.NewV7(), ExpiresAt: expiresAt,
	})
	return err
}

func expiringAt(offset time.Duration) *time.Time {
	moment := grantNoon.Add(offset)
	return &moment
}

func TestAGrantExpiringInThePastIsRefused(t *testing.T) {
	for name, expiresAt := range map[string]*time.Time{
		"an hour ago":      expiringAt(-time.Hour),
		"a second ago":     expiringAt(-time.Second),
		"this very moment": expiringAt(0),
	} {
		err := grantAt(t, expiresAt)
		var detailed *httperr.DetailedError
		if !errors.As(err, &detailed) {
			t.Errorf("%s: err = %v, want a field refusal", name, err)
			continue
		}
		// The FIELD, because a caller fixes what they are pointed at: a bare
		// 422 tells them the request was wrong and not which date to change.
		if len(detailed.Fields) != 1 || detailed.Fields[0].Field != "expires_at" {
			t.Errorf("%s: refusal names %v, want expires_at", name, detailed.Fields)
		}
	}
}

// Equal to now goes with the past, and that is not a rounding preference: the
// predicates are strict (`>`), so an expiry landing exactly on the instant it is
// written is already spent. It is covered above with the other two.

func TestAGrantExpiringInTheFutureOrNeverIsAdmitted(t *testing.T) {
	// A nil pool means the write itself cannot run, so this asserts the shape
	// refusal is not reached — anything that IS returned came from further down.
	for name, expiresAt := range map[string]*time.Time{
		"a second from now": expiringAt(time.Second),
		"next year":         expiringAt(365 * 24 * time.Hour),
		"never":             nil,
	} {
		err := grantAt(t, expiresAt)
		var detailed *httperr.DetailedError
		if errors.As(err, &detailed) && len(detailed.Fields) == 1 && detailed.Fields[0].Field == "expires_at" {
			t.Errorf("%s was refused as an expiry already past: %v", name, err)
		}
	}
}
