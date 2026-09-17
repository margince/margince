// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An erased message must stop answering to its Message-ID.
//
// Erasure ARCHIVES the activity rather than deleting it, so the identity
// table's foreign key never cascades. What is left is both a durable record
// that a message with that identity was here, and a claim nothing can ever
// take back: the claim is ON CONFLICT DO NOTHING, so the dead row holds the
// key forever and every later arrival of the same message files a fresh
// duplicate that can never dedupe against the last one.
//
// So the assertion is the CONSEQUENCE and not the row count: after the
// erasure, the same Message-ID is claimable again.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasedMessageID is the identity the subject's mail claims and the later
// arrival tries to claim back.
const erasedMessageID = "released-by-erasure@example.test"

func TestErasingAContactReleasesItsMessagesIdentities(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	contactID := seedSubject(t, e)

	// The subject's own mail, filed through the real writer so the identity is
	// claimed the way capture and import claim it. No deal or project link and
	// no retention_class, so the commercial-correspondence floor does not
	// shield the row from the redaction this test is about.
	subjectMail := "a-subject@example.test"
	sent, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind:         string(crmcontracts.ActivityKindEmail),
		Source:       "manual",
		EmailFrom:    subjectEmail,
		EmailTo:      []string{subjectMail},
		RFCMessageID: erasedMessageID,
		Links: []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: contactID},
		},
	})
	if err != nil {
		t.Fatalf("filing the subject's mail: %v", err)
	}
	sentID := ids.UUID(sent.Id)
	if holder := identityHolder(t, e, erasedMessageID); holder != sentID {
		t.Fatalf("the mail did not claim its own Message-ID: holder %v, message %v", holder, sentID)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(admin, contactID, "art-17"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	// The claim is gone, so the identity is free.
	if holder := identityHolder(t, e, erasedMessageID); holder != (ids.UUID{}) {
		t.Errorf("the erased message still answers to %q through activity %v — the identity outlived the content it points at",
			erasedMessageID, holder)
	}

	// And free MEANS claimable: the row count is the symptom, this is the
	// consequence. A later arrival of the same message must be able to take
	// the identity, or every future copy is a duplicate nothing can fold.
	arrived, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind:         string(crmcontracts.ActivityKindEmail),
		Source:       "manual",
		EmailFrom:    "someone-else@example.test",
		EmailTo:      []string{"us@example.test"},
		RFCMessageID: erasedMessageID,
	})
	if err != nil {
		t.Fatalf("filing the later arrival: %v", err)
	}
	arrivedID := ids.UUID(arrived.Id)
	if holder := identityHolder(t, e, erasedMessageID); holder != arrivedID {
		t.Errorf("the later arrival could not claim %q — the erased message is still squatting the key, so every copy from here on is a duplicate that can never dedupe",
			erasedMessageID)
	}
}

// identityHolder answers which activity holds a mail identity, or the zero id
// when it is free.
func identityHolder(t *testing.T, e *Env, key string) ids.UUID {
	t.Helper()
	var holder ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		scanErr := tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_identity
			  WHERE identity_kind = 'mail' AND identity_key = $1`, key).Scan(&holder)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			// Free, which is a real answer here and not a missing row: this
			// test is about an identity being released.
			holder = ids.UUID{}
			return nil
		}
		return scanErr
	})
	if err != nil {
		t.Fatalf("reading who holds %q: %v", key, err)
	}
	return holder
}
