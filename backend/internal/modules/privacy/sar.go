// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// GDPR Art. 15 subject-access assembly (admin-mediated in V1): one
// operation gathers everything held about a contact — the normalized
// row, channels, relationships, deals they hold a stake in, timeline
// activities, consent state and proof log, and the raw capture
// payloads that mention them — into a single export package. The
// export is itself audited (action=export): who pulled whose data,
// when.

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssembleSAR builds the package. It is a privileged read: the caller must be
// a human holding the contact.delete grant (the same trust level erasure needs)
// over an unbounded row scope — see the checks below.
func AssembleSAR(ctx context.Context, db *database.DB, contactID ids.ContactID) (SARPackage, error) {
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return SARPackage{}, err
	}
	// Human-only, for the reason the grant above cannot express: an agent
	// acting under a passport carries the granting human's live grants, so an
	// admin's read-scoped passport would otherwise assemble a subject's entire
	// Art. 15 package — every activity, capture row, correction and outbound
	// message.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return SARPackage{}, fmt.Errorf("human-only subject-access assembly: %w", apperrors.ErrPermissionDenied)
	}
	// The assembly deliberately crosses the caller's row scope — Art. 15 owes
	// the subject everything held, not the slice one rep may see — so a bounded
	// caller cannot run it. Scope is the second condition, not a stand-in for
	// authority: the contact.delete grant above is what limits this to the roles
	// trusted with erasure, and it is what keeps read_only out.
	//
	// Together those two admit MORE than admin: the seeded defaults give
	// contact.delete and row scope `all` to ops and management as well. That is
	// this function's contract — the erasure trust level — and callers that owe
	// a narrower one gate before they get here. The subject-request route does:
	// it reads the request through the queue's own admin-only gate first, which
	// TestTheQueueGateIsWhatKeepsTheExportAdminOnly pins from both sides. A
	// future caller assembling from a contact id alone would be reachable by two
	// more roles, and would need its own gate.
	if !auth.Unbounded(actor) {
		return SARPackage{}, apperrors.ErrPermissionDenied
	}
	var pkg SARPackage
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleForSubjectRights(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		// The subject's addresses and lead twins, read BEFORE the sections run:
		// the staged-approvals section matches on them, and unlike erasure this
		// path destroys nothing, so they are still there to read.
		emails, leads, identities, err := subjectReach(ctx, tx, contactID)
		if err != nil {
			return err
		}
		authored, err := subjectAuthoredMessages(ctx, tx, contactID, emails, identities)
		if err != nil {
			return err
		}
		sections := sarSections(&pkg, contactID, emails, leads, identities, authored)

		subject, err := rowMaps(ctx, tx, sarSubjectQuery, contactID)
		if err != nil {
			return err
		}
		if len(subject) == 0 {
			return apperrors.ErrNotFound
		}
		pkg.Subject = subject[0]
		if err := appendSubjectCustomValues(ctx, tx, contactID, pkg.Subject); err != nil {
			return err
		}

		for _, section := range sections {
			args := section.args
			if args == nil {
				args = []any{contactID}
			}
			rows, err := rowMaps(ctx, tx, section.query, args...)
			if err != nil {
				return err
			}
			*section.dest = rows
		}
		if err := appendActivityCustomValues(ctx, tx, pkg.Activities); err != nil {
			return err
		}

		_, err = storekit.Audit(ctx, tx, "export", "contact", contactID.UUID, nil, map[string]any{
			"kind": "sar", "activities": len(pkg.Activities), "raw_rows": len(pkg.RawCapture),
		})
		return err
	})
	return pkg, err
}

// sarSubjectQuery is the subject's own row as the export hands it over. The
// privacy notice page reads the same row to name the kinds it holds.
const sarSubjectQuery = `
	SELECT p.id, p.full_name, p.first_name, p.last_name, p.title,
	       (SELECT jsonb_object_agg(ps.platform, ps.handle) FROM contact_social ps WHERE ps.contact_id = p.id) AS social,
	       p.address_line1, p.address_line2, p.address_city, p.address_region, p.address_postal_code, p.address_country,
	       p.source, p.created_at
	FROM contact p WHERE p.id = $1`

// appendSubjectCustomValues merges the subject's stored cf_ values into
// the export's subject map, keyed by column name. The column set comes
// from the catalog with ANY status (see subjectcolumns.go): Art. 15 owes
// the subject everything HELD, and a retired field's column still stores
// its values. Extraction rides the same storekit mechanics the record
// surface reads with, so each value exports in its documented wire shape;
// a NULL column stays absent, like every other empty section detail.
func appendSubjectCustomValues(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, subject map[string]any) error {
	columns, err := subjectCustomColumns(ctx, tx, "contact")
	if err != nil || len(columns) == 0 {
		return err
	}
	dests := storekit.ScanDests(columns)
	query := `SELECT ` + strings.TrimPrefix(storekit.SelectSuffix(columns), ", ") + ` FROM contact WHERE id = $1`
	if err := tx.QueryRow(ctx, query, contactID).Scan(dests...); err != nil {
		return err
	}
	maps.Copy(subject, storekit.ExtractValues(columns, dests))
	return nil
}

// sarSection pairs a destination package section with the query that fills
// it. Every query is keyed to the single contactID bound param ($1).
type sarSection struct {
	dest  *[]map[string]any
	query string
	// args replaces the default single contact-id argument.
	args []any
}

// rowMaps runs one query and returns each row as column→value.
func rowMaps(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(values))
		for i, field := range rows.FieldDescriptions() {
			row[field.Name] = readableValue(values[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// readableValue renders one column value as the document's reader needs it.
//
// A uuid column arrives from pgx as [16]byte, and encoding/json has one
// rendering for a byte ARRAY: sixteen numbers. Nothing is lost — the bytes are
// the id — but the document this builds is what a data subject receives under
// Art. 15, and `[1,160,94,189,…]` is not something a contact can read, act on,
// or quote back when they dispute it.
//
// Fixed HERE rather than at the two columns that showed it, because those two
// are not a property of those columns: every map-shaped section is built by
// this loop, so the next uuid column added to any of the twenty-six arrives
// with the same defect. TestTheSubjectAccessPackageIsReadable holds it.
//
// Anything else is passed through untouched. This is a rendering decision about
// one Go type that JSON has no good answer for, not a place to reinterpret what
// the database returned — a uuid[] column would arrive as [][16]byte and want
// the same treatment, and gets it in the change that adds one rather than as an
// arm nothing exercises.
//
//craft:ignore naked-any a column value IS any by construction — pgx's rows.Values() returns []any, and this rewrites one of those in place. A concrete parameter would mean deciding the column's type before reading it, which is the thing the caller cannot do.
func readableValue(value any) any {
	if raw, isUUID := value.([16]byte); isUUID {
		return ids.UUID(raw).String()
	}
	return value
}

// subjectReach reads the two things beyond a contact id that identify the
// subject in a staged proposal: the addresses a message to them carries, and
// the lead rows that were the same human before promotion.
//
// Read here rather than derived inside the query so the export can hand the
// SAME bound parameters to subjectApprovalMatch that the erasure hands it.
// Erasure cannot read them at that point — it has already destroyed them — so
// the predicate takes them as arguments and each caller supplies them from
// wherever it still can.
func subjectReach(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]string, []ids.UUID, []ids.UUID, error) {
	emails, err := subjectStrings(ctx, tx,
		`SELECT email FROM contact_email WHERE contact_id = $1 AND email <> ''`, contactID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading the subject's addresses for the export: %w", err)
	}
	// THE ADDRESSES OF EVERY IDENTITY THIS SUBJECT IS, not only the surviving
	// row's. A merged-away predecessor keeps its own contact_email rows, and the
	// sections that match by address — the participant reach above, staged
	// approvals — would otherwise miss a conversation held under the address
	// the subject used before the cleanup.
	priorEmails, err := subjectStrings(ctx, tx, `
		SELECT email FROM contact_email
		 WHERE contact_id IN (SELECT id FROM contact WHERE merged_into_id = $1)
		   AND email <> ''`, contactID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading the merged-away addresses for the export: %w", err)
	}
	emails = append(emails, priorEmails...)
	// LEAD TWINS OF EVERY IDENTITY TOO. A lead promoted into a record that was
	// later merged away points at the PREDECESSOR, so following only the
	// survivor's promotions loses the lead-keyed proof A6 deliberately leaves
	// on the lead.
	rows, err := tx.Query(ctx, `
		SELECT id FROM lead
		 WHERE promoted_contact_id = $1
		    OR promoted_contact_id IN (SELECT id FROM contact WHERE merged_into_id = $1)`,
		contactID.UUID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading the subject's lead twins for the export: %w", err)
	}
	defer rows.Close()
	leads := []ids.UUID{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, nil, nil, err
		}
		leads = append(leads, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	// EVERY CONTACT ID THIS SUBJECT IS. The surviving row first, then each
	// record merged into it — which keeps its own rows on purpose, because a
	// predecessor's objection is evidence that THAT record's subject refused,
	// and repointing it would make the history say somebody else objected.
	//
	// One hop, not a walk: a merge repoints the loser's own predecessors at the
	// survivor as it goes, so the pointers are flat rather than a chain.
	identities, err := subjectIdentities(ctx, tx, contactID)
	if err != nil {
		return nil, nil, nil, err
	}
	return emails, leads, identities, nil
}

// subjectIdentities returns the survivor's own id plus the id of each record
// merged into them, which together are the contact rows an export must reach.
func subjectIdentities(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]ids.UUID, error) {
	// The survivor's own id is carried in the WHERE rather than selected as a
	// bare parameter: a lone `SELECT $1` in a UNION gives Postgres nothing to
	// infer the type from, and it refuses to prepare the statement.
	rows, err := tx.Query(ctx, `
		SELECT id FROM contact WHERE id = $1 OR merged_into_id = $1`, contactID.UUID)
	if err != nil {
		return nil, fmt.Errorf("reading the identities this subject is, for the export: %w", err)
	}
	defer rows.Close()
	out := []ids.UUID{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// subjectStrings runs a one-column text query into a slice. The contact is $1,
// and any further arguments follow it.
func subjectStrings(ctx context.Context, tx pgx.Tx, query string, contactID ids.ContactID, more ...any) ([]string, error) {
	rows, err := tx.Query(ctx, query, append([]any{contactID.UUID}, more...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
