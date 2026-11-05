// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
// its tables. Contacts archives them through its own writer, so the write shape
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
		retracted, err := retractCapturedContactTx(ctx, tx, e.contacts, contact.ContactID, contact.OwnerID)
		if err != nil {
			return err
		}
		if !retracted {
			// A record a human touched, or one already promoted to the
			// workspace. Left alone on purpose — see RetractCaptureOnlyContactTx
			// for why each of those outranks a classifier's opinion about one
			// conversation.
			continue
		}
		e.log.InfoContext(ctx, "confidentiality: retracted a contact a private thread orphaned",
			"contact", contact.ContactID.String())
	}
	return retractPrivateLeadLinksTx(ctx, tx, row.ThreadKey, row.UserID)
}

// The evidence a lead link taken off private mail records, by the verdict that
// made it private.
const (
	privateThreadLeadReason  = "personal_thread_verdict"
	personalSenderLeadReason = "personal_sender_verdict"
)

// retractPrivateLeadLinksTx takes off the lead links capture filed for messages
// a personal thread verdict has made private.
func retractPrivateLeadLinksTx(ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID) error {
	links, err := capture.LeadLinksOrphanedByPrivacyTx(ctx, tx, threadKey, user)
	if err != nil {
		return err
	}
	return retractLeadLinksTx(ctx, tx, links, user, privateThreadLeadReason)
}

// retractPersonalSenderLeadLinksTx takes off the lead links capture filed for
// a sender's messages that a personal sender verdict has made private.
func retractPersonalSenderLeadLinksTx(ctx context.Context, tx pgx.Tx, address string, user ids.UUID) error {
	links, err := capture.LeadLinksOfPersonalSenderTx(ctx, tx, address, user)
	if err != nil {
		return err
	}
	return retractLeadLinksTx(ctx, tx, links, user, personalSenderLeadReason)
}

// retractLeadLinksTx writes each removal through the activities writer, so it
// lands its audit row. A link a human filed stays.
func retractLeadLinksTx(ctx context.Context, tx pgx.Tx, links []capture.PrivateLeadLink, user ids.UUID, reason string) error {
	for _, link := range links {
		if _, err := activities.RetractMachineLeadTx(ctx, tx, link.ActivityID, link.LeadID,
			map[string]any{"reason": reason, "decided_as": user}); err != nil {
			return err
		}
	}
	return nil
}

// retractCapturedContactTx withdraws one capture-made contact and ends the
// notice duties it owed, in the caller's transaction, so a withdrawn record
// leaves no duty on the worklist.
func retractCapturedContactTx(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, id ids.ContactID, owner ids.UUID,
) (bool, error) {
	retracted, err := store.RetractCaptureOnlyContactTx(ctx, tx, id, owner)
	if err != nil || !retracted {
		return retracted, err
	}
	_, err = consent.SettleWhenCaptureWithdrewTx(ctx, tx, id.UUID, time.Now().UTC())
	return true, err
}
