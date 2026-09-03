// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// retractPrivateContactsTx withdraws the contacts a personal verdict has just
// orphaned.
//
// The verdict almost always arrives AFTER the contact. Capture creates on
// commit and classification reads the thread later, so refusing at creation
// time catches only what arrives afterwards. In one real mailbox every contact
// on a personal thread — all forty-six of them, a founder's aunt among them —
// predated the verdict about it. Without this the fix would have prevented
// nothing that had already happened.
//
// The two halves stay in their own modules. Capture answers which records a
// private thread orphaned, because the thread ledger and the activity rows are
// its tables. People archives them through its own writer, so the write shape
// holds and a retraction lands an audit row exactly like a human's archive.
// This is the seam that joins them, in the SAME transaction as the verdict:
// a retraction that committed separately could leave a thread judged personal
// with its contact still standing if the second write failed.
func (e *ConfidentialityVerdictEngine) retractPrivateContactsTx(
	ctx context.Context, tx pgx.Tx, row capture.PendingThread, kind string,
) error {
	if kind != capture.ThreadKindPersonal {
		return nil
	}
	orphaned, err := capture.ContactsOrphanedByPrivacyTx(ctx, tx, row.ThreadKey, row.UserID)
	if err != nil {
		return err
	}
	for _, contact := range orphaned {
		retracted, err := e.people.RetractCaptureOnlyPersonTx(ctx, tx, contact.PersonID, contact.OwnerID)
		if err != nil {
			return err
		}
		if !retracted {
			// A record a human touched, or one already promoted to the
			// workspace. Left alone on purpose — see RetractCaptureOnlyPersonTx
			// for why each of those outranks a classifier's opinion about one
			// conversation.
			continue
		}
		e.log.InfoContext(ctx, "confidentiality: retracted a contact a private thread orphaned",
			"person", contact.PersonID.String())
	}
	return nil
}
