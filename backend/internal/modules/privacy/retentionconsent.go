// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The live capabilities over an anonymized subject's consent record.
//
// Its own file for the size reason retentionactions.go's neighbours have one,
// and named after the Art. 17 twin it mirrors (erasure_consent.go): the two
// lists must hold the same credentials, and a fifth added to one and not the
// other is the drift contactscrub_test.go exists to catch.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteConsentCredentials removes the live capabilities over one subject's
// consent record: the two tokens that authorise a grant, and the link that
// displays it.
//
// Together rather than four blocks inside the anonymize, because they answer to
// one argument — an anonymized subject may lawfully RETURN, and they return by
// being invited again, not by an old credential in an old mailbox still
// working. The Art. 17 twin of this list has its own file for the same reason
// (erasure_consent.go), and a fourth credential added to one and not the other
// is the drift contactscrub_test.go exists to catch.
//
// What came BACK through the confirm link stays in retentionactions.go beside
// the executor, and deliberately: contact_confirm_submission is a contact
// satellite, and satellite_lifecycle_test.go reads the anonymize's obligations
// out of that file. Moving it here would leave that census reading a corpus
// without it and reporting PASS over a satellite nobody deletes.
func deleteConsentCredentials(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// One loop rather than three blocks, because the three are one act: the
	// error handling is identical and spelling it per statement made the
	// reason for each credential harder to see, not easier.
	for _, statement := range []string{
		// The double-opt-in token goes with the addresses it was sent to. It is
		// a bearer secret whose only function is to authorise a consent GRANT
		// for this subject, so one left standing after an anonymization is a
		// live invitation to record a lawful basis for somebody the row no
		// longer names. An anonymized subject may lawfully return, which is
		// what the suppression list is for — but they return by being invited
		// again, not by an old token in an old mailbox still working.
		`DELETE FROM consent_doi_token WHERE contact_id = $1`,
		// The confirm-details link goes for the same reason, and a stronger
		// one: it does not merely authorise a grant, it DISPLAYS the record. A
		// link left live would show an old mailbox the fields the anonymize has
		// just emptied.
		`DELETE FROM confirm_token WHERE contact_id = $1`,
		// The withdrawal link goes too, and it is the one that would linger
		// longest: 24 months against the double-opt-in token's weeks. It also
		// HOLDS THE ADDRESS the link was written to, in its own column, which
		// is precisely the content the anonymize just cleared from the contact
		// row — so leaving it would keep an anonymized subject's mailbox
		// legible in a table the anonymization did not touch.
		`DELETE FROM withdrawal_credential WHERE contact_id = $1`,
	} {
		if _, err := tx.Exec(ctx, statement, id); err != nil {
			return err
		}
	}
	return nil
}
