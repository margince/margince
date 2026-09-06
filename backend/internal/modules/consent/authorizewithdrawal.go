// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The subject's own recorded stop, read where the evidence arms can see it.
//
// A withdrawal is Art. 7(3): the person takes back a permission they gave. It
// is written to person_consent and NOT to communication_suppression, so
// liveSuppression cannot see it — and the evidence arms allow on the record's
// own ground without ever reading person_consent, which is exactly what lets a
// reply to a thread the subject started work with no consent row.
//
// Those two facts together were a leak: somebody who wrote into a thread and
// then pressed one-click unsubscribe was still sent the next message on it,
// because the thread arm answered before anything read the withdrawal. The
// dispatcher used to catch it by asking the legacy purpose gate a second time
// after the ticket already said yes; that second gate is gone, so the engine
// asks the question itself.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// withdrawalCovers reports whether the subject has withdrawn a purpose whose
// class covers this category.
//
// BY CLASS, not by the caller's purpose key. The evidence arms resolve a
// category from the record and may have no purpose key at all, and an
// unsubscribe-all withdraws every unlocked purpose rather than one — so the
// question is "did they stop the kind of message this is", which is what a
// subject pressing unsubscribe believes they answered.
func withdrawalCovers(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category) (bool, error) {
	classes := classesCovering(category)
	if len(classes) == 0 {
		return false, nil
	}
	// person_consent carries person_id OR lead_id, so one statement answers for
	// both subject kinds rather than a second copy answering for one.
	var withdrawn bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM person_consent pc
			  JOIN consent_purpose cp ON cp.id = pc.purpose_id
			 WHERE (($1 = 'person' AND pc.person_id = $2::uuid)
			     OR ($1 = 'lead'   AND pc.lead_id   = $2::uuid))
			   AND pc.state = 'withdrawn'
			   -- ARCHIVED PURPOSES COUNT. A withdrawal is a thing the subject
			   -- did, and archiving the purpose is a thing the installation
			   -- did; letting the second erase the first would make retiring a
			   -- purpose reactivate everybody who had stopped it.
			   AND cp.class = ANY($3)
		)`, subject.Kind, subject.ID, classes).Scan(&withdrawn); err != nil {
		return false, fmt.Errorf("consent: read whether the subject stopped this kind of message: %w", err)
	}
	return withdrawn, nil
}

// classesCovering inverts categoryForClass: which purpose classes produce this
// category, and so which withdrawals speak about it.
//
// The five subject-serving categories return none. A person cannot withdraw
// their way out of a security warning or the acknowledgement of their own
// opt-out, and reading a withdrawal as covering those would make pressing
// unsubscribe silence the confirmation that it worked.
func classesCovering(category commsauthz.Category) []string {
	if category.ServesTheSubject() {
		return nil
	}
	switch category {
	case commsauthz.CategoryMarketing:
		return []string{string(ClassMarketing), string(ClassPhoneOutreach)}
	case commsauthz.CategoryAccountNotice, commsauthz.CategoryContractNotice,
		commsauthz.CategoryInvoiceOrPayment:
		// Transactional messages rest on the contract, not on consent, so a
		// withdrawal does not reach them: somebody who unsubscribes is still
		// owed their invoice.
		return nil
	default:
		return []string{string(ClassBusinessCorrespondence)}
	}
}
