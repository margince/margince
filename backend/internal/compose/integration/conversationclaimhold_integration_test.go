// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The claim writer holds its subject rather than merely probing it.
//
// `RecordConversationClaim` asked `EnsureWritableLive` and then wrote on a
// later statement. The probe reads a snapshot, so an Art. 17 erasure committing
// between the two would put a claim — and the verbatim sentence on it — back
// against a contact the operator had just been told was gone.
//
// The window itself cannot be opened from a test without freezing the writer
// mid-transaction, and the row lock is what closes it. What CAN be asserted is
// that the hold is a live-row hold: a contact already retired is refused rather
// than written against, which is the same predicate the race turns on and the
// half a reader can see.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAClaimIsRefusedAgainstARetiredContact(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	owner := OwnerConn(t)
	contact := e.SeedContact(t, "Ute Sommer", nil)
	activity := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Re: Angebot', 'I will send the signed order by Friday.',
		        now(), 'capture', 'connector:t')`, activity); err != nil {
		t.Fatalf("seeding the message: %v", err)
	}
	LinkActivity(t, owner, activity, "contact", contact)

	in := contacts.ClaimInput{
		ContactID:  ids.From[ids.ContactKind](contact),
		Kind:       "commitment_theirs",
		Body:       "Send the signed order by Friday",
		ActivityID: activity,
		Quote:      "I will send the signed order by Friday.",
		Source:     "manual",
	}
	// The control: it lands while the contact is live, so the refusal below is
	// about the retirement and not about the fixture.
	if _, err := e.Contacts.RecordConversationClaim(ctx, in); err != nil {
		t.Fatalf("a claim on a live contact was refused, so this fixture proves nothing: %v", err)
	}

	if _, err := owner.Exec(context.Background(),
		`UPDATE contact SET archived_at = now() WHERE id = $1`, contact); err != nil {
		t.Fatalf("retiring the contact: %v", err)
	}

	if _, err := e.Contacts.RecordConversationClaim(ctx, in); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a claim against a retired contact answered %v, want not-found — the row carries a "+
			"verbatim sentence, so one written after the record was retired is the subject's own "+
			"words landing back beside a record that no longer names them", err)
	}
}
