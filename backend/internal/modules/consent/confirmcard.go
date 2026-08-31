// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the confirm page shows a person about themselves.
//
// A purpose-built projection, never the Person360 read model. That one carries
// internal fields — owner, lifecycle, scores, the research trail — which are
// this workspace's working notes about a contact rather than the contact's own
// data, and which no subject asked to be shown. This read names its columns
// one at a time so a field added to the person record cannot arrive on a public
// page by inheritance.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Field names the confirm page uses, and the closed set a correction may name.
// Spelled as constants because they cross the wire twice — the card names them,
// a correction quotes one back — and a typo on either side would silently stage
// a proposal against a field nobody reads.
const (
	ConfirmFieldFullName = "full_name"
	ConfirmFieldTitle    = "title"
	ConfirmFieldEmail    = "email"
	ConfirmFieldPhone    = "phone"
)

// confirmCorrectableFields is the closed set a submission may name. The company
// is deliberately absent: which organization employs somebody is a relationship
// this workspace maintains, not a string on the person, and a correction to it
// would have to create or merge a company record — which is a rep's judgment
// and not a text box on a public page.
var confirmCorrectableFields = map[string]bool{
	ConfirmFieldFullName: true,
	ConfirmFieldTitle:    true,
	ConfirmFieldEmail:    true,
	ConfirmFieldPhone:    true,
}

// ConfirmCard is one person's own view of what is held about them.
type ConfirmCard struct {
	FullName string
	Title    string
	Company  string
	Email    string
	Phone    string
	// Provenance answers Art. 14 per field: where this value came from and when
	// it was recorded. Empty for a field nothing has stamped.
	Provenance []FieldOrigin
	// Marketing is the subject's current answer, so a person who already said
	// yes is not asked as though they had not.
	Marketing string
}

// FieldOrigin is one line of "where we got this".
type FieldOrigin struct {
	Field      string
	Source     string
	RecordedAt string
}

// ConfirmCardFor reads the card behind one confirm link. It runs outside
// row-level security like every other read on this surface: there is no session,
// and the token the caller already redeemed IS the authority.
//
// The caller must have resolved a live token for this person. Nothing here
// re-checks that, which is why it is unexported to the transport and reached
// only from the handler that resolves first.
func (s *Store) confirmCardFor(ctx context.Context, personID ids.PersonID) (ConfirmCard, error) {
	var card ConfirmCard
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		// The employer comes through the live employment relationship rather
		// than a column: a person's company is a relationship, and reading it
		// any other way would show a company they have left.
		//
		// The currency test is a DATE comparison and not a null check, matching
		// people.EmploymentIsCurrentSQL exactly. Somebody serving three months'
		// notice still works there, and a null check would take their employer
		// off their own card the day the notice was filed. Spelled here rather
		// than called because a module may not import a sibling; the copy is
		// ratified in the currency census with that reason.
		err := tx.QueryRow(ctx, `
			SELECT coalesce(p.full_name, ''), coalesce(p.title, ''),
			       coalesce((SELECT o.display_name FROM relationship r
			                   JOIN organization o ON o.id = r.organization_id
			                  WHERE r.person_id = p.id AND r.kind = 'employment'
			                    AND (r.ended_at IS NULL OR r.ended_at > current_date)
			                    AND r.archived_at IS NULL
			                  ORDER BY r.created_at DESC LIMIT 1), ''),
			       `+primaryEmailSQL("p.id")+`,
			       coalesce((SELECT pp.phone FROM person_phone pp
			                  WHERE pp.person_id = p.id AND pp.archived_at IS NULL
			                  ORDER BY pp.is_primary DESC, pp.created_at LIMIT 1), '')
			  FROM person p
			 WHERE p.id = $1 AND p.archived_at IS NULL`,
			personID).Scan(&card.FullName, &card.Title, &card.Company, &card.Email, &card.Phone)
		if err != nil {
			return err
		}
		card.Provenance, err = fieldOriginsFor(ctx, tx, personID)
		if err != nil {
			return err
		}
		// No answer on record reads as empty rather than as an error: a person
		// who has never been asked is the ordinary case on this page.
		err = tx.QueryRow(ctx, `
			SELECT pc.state
			  FROM person_consent pc
			  JOIN consent_purpose cp ON cp.id = pc.purpose_id
			 WHERE pc.person_id = $1 AND cp.key = $2`,
			personID, PurposeMarketingEmail).Scan(&card.Marketing)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return card, err
}

// fieldOriginsFor answers the Art. 14 question the page puts behind a
// disclosure: for each thing we hold, where did it come from and when.
//
// This is the strongest part of the page and it costs nothing to render,
// because the provenance rows already exist — every capture path stamps them.
// Most CRMs cannot answer it at all.
func fieldOriginsFor(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]FieldOrigin, error) {
	rows, err := tx.Query(ctx, `
		SELECT field_name, source, to_char(captured_at, 'YYYY-MM-DD')
		  FROM field_provenance
		 WHERE object_type = 'person' AND object_id = $1
		 ORDER BY field_name`, personID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FieldOrigin, error) {
		var o FieldOrigin
		return o, row.Scan(&o.Field, &o.Source, &o.RecordedAt)
	})
}
