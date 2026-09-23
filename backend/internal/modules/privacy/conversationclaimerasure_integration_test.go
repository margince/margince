// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The subject-scoped arm of the claim purge, against real rows.
//
// A claim carries a verbatim snippet of the message it was read from, so the
// row is a copy of somebody's own words held outside that message. The schema
// means the contact foreign key to take it away — it is NOT NULL and cascades —
// but both acts ANONYMIZE the contact row in place rather than deleting it, so
// nothing ever fires. That is the whole reason this is a statement.
//
// Both acts are asked, because they are two writers of one invariant: an
// Art. 17 request and the retention sweep's contact/anonymize clear the same
// rows, and a helper wired into one alone leaves the other keeping what the
// first was built to remove.
//
// Three claims per case, one for each way a row can belong to the subject, so
// neither arm of the statement can be dropped and still pass. A claim ABOUT the
// subject read from somebody else's message goes by contact_id; a COLLEAGUE'S
// claim quoting the subject's message goes by the activity walk; the subject's
// own claim on their own message is reached by either. A purge with only the
// walk leaves a claim naming the subject standing, and one with only the
// contact key leaves a sentence the subject wrote inside somebody else's row.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// claimErasers are the two subject-scoped acts that must destroy the claims.
var claimErasers = map[string]func(context.Context, pgx.Tx, ids.ContactID) error{
	"an Art. 17 request": deleteConversationClaimsFor[ids.ContactID],
	"the retention sweep's anonymize": func(ctx context.Context, tx pgx.Tx, contact ids.ContactID) error {
		return deleteConversationClaimsFor(ctx, tx, contact.UUID)
	},
}

func TestBothActsDestroyTheClaimsReadOutOfASubjectsConversations(t *testing.T) {
	for name, erase := range claimErasers {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := subjectColumnsTx(ctx, t)

			ws, user := ids.NewV7(), ids.NewV7()
			subject, colleagueContact := ids.New[ids.ContactKind](), ids.New[ids.ContactKind]()
			activity, colleagueActivity := ids.NewV7(), ids.NewV7()
			mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
			mustExec(ctx, t, tx,
				`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
				user, "admin-"+user.String()+"@anon.test")
			for _, contact := range []ids.ContactID{subject, colleagueContact} {
				mustExec(ctx, t, tx,
					`INSERT INTO contact (id, full_name, source, captured_by)
					 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, contact, user)
			}
			for _, id := range []ids.UUID{activity, colleagueActivity} {
				mustExec(ctx, t, tx,
					`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
					 VALUES ($1, 'email', 'Re: Angebot', 'I will send the signed order by Friday.',
					         now(), 'capture', 'connector:t')`, id)
			}
			mustExec(ctx, t, tx,
				`INSERT INTO activity_link (activity_id, entity_type, contact_id)
				 VALUES ($1, 'contact', $2)`, activity, subject)
			mustExec(ctx, t, tx,
				`INSERT INTO activity_link (activity_id, entity_type, contact_id)
				 VALUES ($1, 'contact', $2)`, colleagueActivity, colleagueContact)
			// One row per way of belonging to the subject: their own claim on
			// their own message, a colleague's claim quoting it, and a claim
			// ABOUT the subject read from a message that is not theirs.
			claims := []struct {
				contact  ids.ContactID
				activity ids.UUID
			}{
				{subject, activity},
				{colleagueContact, activity},
				{subject, colleagueActivity},
			}
			for _, claim := range claims {
				mustExec(ctx, t, tx,
					`INSERT INTO conversation_claim
					     (contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by)
					 VALUES ($1, 'commitment_theirs', 'Send the signed order by Friday', $2,
					         'I will send the signed order by Friday.', md5($4),
					         'manual', 'user:'||$3::text)`,
					claim.contact, claim.activity, user,
					claim.contact.String()+":"+claim.activity.String())
			}

			if err := erase(ctx, tx, subject); err != nil {
				t.Fatalf("erasing the subject's claims: %v", err)
			}

			var left int
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM conversation_claim`).Scan(&left); err != nil {
				t.Fatalf("counting the claims left behind: %v", err)
			}
			if left != 0 {
				t.Errorf("%d claim(s) still quote the erased subject's message — the words were "+
					"destroyed in one place and kept verbatim in another", left)
			}
		})
	}
}
