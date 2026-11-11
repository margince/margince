// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The working-record half of an Art. 17 erasure: what a colleague's own record
// says about the subject in passing. It includes every table another module or
// a unit declared for the erasure to reach.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/sweepreach"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactWorkingRecords clears the subject's name where it survives inside
// somebody else's working record. Two are an automation run addressed to them
// and a voice sample that names them. The others are a commitment on a
// colleague's weekly plan and an introduction one colleague asked another for.
//
// The four are one step because they answer one question. What does an erasure
// owe a record that is not the subject's, but names them in passing?
func (e *Eraser) redactWorkingRecords(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keys subjectKeys) error {
	if err := redactWorkflowRuns(ctx, tx, keys.emails); err != nil {
		return err
	}
	if err := e.purgeTextNaming(ctx, tx, keys); err != nil {
		return err
	}
	if err := redactDeclaredTables(ctx, tx, subject); err != nil {
		return err
	}
	// And what they wrote about the subject while asking each other for an
	// introduction, which is the same shape one table further out
	// (erasureintroductions.go).
	return redactIntroductionRequests(ctx, tx, subject)
}

// erasedMention is what a colleague's own record says instead of the subject's name.
// It is a tombstone for two reasons. The tables refuse an empty value
// (weekly_plan_commitment_label_present, intro_request_reason_present). And the
// colleague's record may still say that something was there.
const erasedMention = "(erased: the contact this named exercised erasure)"

// redactDeclaredTables clears what the declared tables (sweepreach) say about
// the subject, in the single erasure transaction, and tombstones each cleared
// row. A weekly plan commitment is one: a rep's free text about a contact that
// no foreign key ties to the subject. The owner names the columns, because only
// it knows which of them its constraints bind together.
func redactDeclaredTables(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	cleared, err := sweepreach.Erase(ctx, tx, contactID.UUID, erasedMention)
	if err != nil {
		return fmt.Errorf("privacy: redacting the declared tables naming the subject: %w", err)
	}
	for _, c := range cleared {
		if err := tombstoneCollateralScrubs(ctx, tx, c.Table, c.Rows, subjectRedactionReason, subjectRedactionCause); err != nil {
			return err
		}
	}
	return nil
}
