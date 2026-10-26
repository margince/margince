// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What an accepted LinkedIn match may write besides the profile link. The
// export's position can become the contact's title. The company the import
// placed the connection at can become their current employer. Each write needs
// a box the human ticked, so a re-import never writes either.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// LinkedInMatchFields is what a match card says about the two optional writes,
// and which of them the human chose.
type LinkedInMatchFields struct {
	// Position is the export's position: the value "Use as title" writes, and
	// the role of the employment "Use as employer" creates.
	Position string
	// ContactTitle is the contact's title when the card was staged. The apply
	// refuses a title write when the title has changed since, so a human edit
	// made while the card waited is never replaced.
	ContactTitle string
	// EmployerCompanyID is the existing company the import placed the
	// connection at. Nil means the card offers no employer.
	EmployerCompanyID *ids.UUID
	// EmployerRelationshipID is the contact's current primary employment when
	// the card was staged, nil for none. It plays ContactTitle's part for the
	// employer.
	EmployerRelationshipID *ids.UUID
	UseAsTitle             bool
	UseAsEmployer          bool
}

// LinkedInMatchApply is one accepted match: the pair the card was staged for,
// and what the human ticked on it.
type LinkedInMatchApply struct {
	ConnectionID ids.UUID
	// OwnerUserID is zero for a card staged before the field existed; the
	// apply then reads it off the connection.
	OwnerUserID ids.UUID
	ContactID   ids.UUID
	Fields      LinkedInMatchFields
	// Columns is the contact catalog the title write reads. The caller fetches
	// it with ActiveContactUpdateColumns before it opens the transaction.
	Columns CustomColumns
}

// linkedInFieldsWritten says which of the two optional writes changed a row,
// for the decision event.
type linkedInFieldsWritten struct{ title, employer bool }

// ActiveContactUpdateColumns is ActiveContactColumns for a caller-opened
// contact update. It takes contact:update, the grant the write it feeds takes.
func (s *Store) ActiveContactUpdateColumns(ctx context.Context) (CustomColumns, error) {
	return s.activeCustomColumns(ctx, entityContact, principal.ActionUpdate)
}

// stagedOffer is the default each box is staged with: ticked only when the
// contact has nothing there yet. A box shown unticked still writes when the
// human ticks it.
func stagedOffer(f LinkedInMatchFields) LinkedInMatchFields {
	f.UseAsTitle = f.Position != "" && f.ContactTitle == ""
	f.UseAsEmployer = f.EmployerCompanyID != nil && f.EmployerRelationshipID == nil
	return f
}

// linkedInOfferColumns is the SELECT list a pending match reads for its card,
// after the six columns the match itself takes. The aliases are `c` for the
// connection, `p` for the contact, and `cur` for the contact's current primary
// employment, which linkedInCurrentEmploymentJoin supplies.
//
// The employer half takes the stager's own grants. The matched company is
// offered only when they may see it. The current employment counts as known
// only when they may see that edge and its company. A card never carries an id
// its stager could not have read.
func linkedInOfferColumns(ctx context.Context, arg func(any) int) (string, error) {
	edgeScope, visible, err := employerScope(ctx, "rel", arg)
	if err != nil {
		return "", err
	}
	if !visible {
		return `, coalesce(c.position, ''), coalesce(p.title, ''), NULL::uuid, cur.id, false, false`, nil
	}
	companyScope, err := auth.ScopeClauseFor(ctx, companyEntity, "mc", arg)
	if err != nil {
		return "", err
	}
	if companyScope == "" {
		companyScope = scopeAllRows
	}
	return storekit.SQLf(`, coalesce(c.position, ''), coalesce(p.title, ''),
		(SELECT mc.id FROM company mc WHERE mc.id = c.matched_company_id AND mc.archived_at IS NULL AND %s),
		cur.id, coalesce(cur.company_id = c.matched_company_id, false),
		cur.id IS NULL OR EXISTS (SELECT 1 FROM relationship rel JOIN company company ON company.id = rel.company_id
		                           WHERE rel.id = cur.id AND %s)`, companyScope, edgeScope), nil
}

// linkedInCurrentEmploymentJoin joins the contact's current primary employment
// as `cur`. uq_rel_current_primary_employer keeps it to one row.
var linkedInCurrentEmploymentJoin = `
	LEFT JOIN LATERAL (SELECT r.id, r.company_id FROM relationship r
	                    WHERE r.contact_id = p.id AND ` + employment.CurrentPrimarySlotSQL("r") + `) cur ON true`

// scanLinkedInOffer reads what linkedInOfferColumns selected into the card's
// fields, with each box at its staged default.
func scanLinkedInOffer(scan func(dest ...any) error, head ...any) (LinkedInMatchFields, error) {
	var f LinkedInMatchFields
	var matched, current *ids.UUID
	var primaryThere, currentVisible bool
	if err := scan(append(head, &f.Position, &f.ContactTitle, &matched, &current, &primaryThere, &currentVisible)...); err != nil {
		return f, err
	}
	// A contact whose current employer is already the matched company has
	// nothing to gain, and an employment the stager cannot see cannot be the
	// snapshot.
	if matched != nil && !primaryThere && currentVisible {
		f.EmployerCompanyID, f.EmployerRelationshipID = matched, current
	}
	return stagedOffer(f), nil
}

// CheckLinkedInMatchFields asks the apply's own refusals while the card can
// still be decided again. A human whose box no longer fits can then untick it.
// The apply asks again under the contact lock, which is what keeps a value
// from being replaced.
func (s *Store) CheckLinkedInMatchFields(ctx context.Context, in LinkedInMatchApply) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return checkLinkedInMatchFields(ctx, tx, in)
	})
}

// checkLinkedInMatchFields refuses a ticked box before anything is written.
// It refuses when the export or the contact moved since the card was staged,
// and when the decider may not make the employer write.
func checkLinkedInMatchFields(ctx context.Context, tx pgx.Tx, in LinkedInMatchApply) error {
	f := in.Fields
	if !f.UseAsTitle && !f.UseAsEmployer {
		return nil
	}
	var position string
	var matchedCompany *ids.UUID
	// Locked, so an import cannot move the position or the company between this
	// check and the write. The contact is held already, which is the order the
	// erasure takes.
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(position, ''), matched_company_id FROM linkedin_connection
		  WHERE id = $1 AND tombstoned_at IS NULL FOR UPDATE`,
		in.ConnectionID).Scan(&position, &matchedCompany); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("contacts: reading the connection an accepted match names: %w", err)
	}
	if position != f.Position {
		return fmt.Errorf("the LinkedIn export changed this connection's position after the match was proposed; "+
			"read the proposal again: %w", apperrors.ErrConflict)
	}
	if f.UseAsTitle {
		if err := checkStagedTitle(ctx, tx, in.ContactID, f.ContactTitle); err != nil {
			return err
		}
	}
	if f.UseAsEmployer {
		return checkStagedEmployer(ctx, tx, in.ContactID, f, matchedCompany)
	}
	return nil
}

func checkStagedTitle(ctx context.Context, tx pgx.Tx, contactID ids.UUID, staged string) error {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return err
	}
	var title string
	if err := tx.QueryRow(ctx, `SELECT coalesce(title, '') FROM contact WHERE id = $1`, contactID).Scan(&title); err != nil {
		return fmt.Errorf("contacts: reading the title an accepted match would replace: %w", err)
	}
	if title != staged {
		return fmt.Errorf("the contact's title changed after this match was proposed; read the proposal again: %w",
			apperrors.ErrConflict)
	}
	return nil
}

// checkStagedEmployer holds the employer box to what the card showed. It also
// holds it to who may write it: sight of the company, and the grant for the
// write the box makes (see requireEmployerWrite).
func checkStagedEmployer(ctx context.Context, tx pgx.Tx, contactID ids.UUID, f LinkedInMatchFields, matched *ids.UUID) error {
	if f.EmployerCompanyID == nil || matched == nil || *matched != *f.EmployerCompanyID {
		return fmt.Errorf("the company this connection was placed at changed after the match was proposed; "+
			"read the proposal again: %w", apperrors.ErrConflict)
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, companyEntity, *f.EmployerCompanyID); err != nil {
		return err
	}
	// The contact's employment lock, which every employment create and update
	// takes, so the edges read here are the ones the write changes.
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contactID.String()); err != nil {
		return err
	}
	live, err := liveEmploymentAt(ctx, tx, contactID, *f.EmployerCompanyID)
	if err != nil {
		return err
	}
	if err := requireEmployerWrite(ctx, live); err != nil {
		return err
	}
	var current *ids.UUID
	err = tx.QueryRow(ctx, `SELECT r.id FROM relationship r WHERE r.contact_id = $1 AND `+
		employment.CurrentPrimarySlotSQL("r"), contactID).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("contacts: reading the employment an accepted match would replace: %w", err)
	}
	if !sameOptionalID(current, f.EmployerRelationshipID) {
		return fmt.Errorf("the contact's current employer changed after this match was proposed; "+
			"read the proposal again: %w", apperrors.ErrConflict)
	}
	return nil
}

// requireEmployerWrite is the grant the employer box needs. A new employment
// is a create. Promoting one the contact already holds changes that edge, so
// it is an update: a grant to create never lets a decider edit an edge.
func requireEmployerWrite(ctx context.Context, live *liveEmployment) error {
	if live == nil {
		return requireRelationshipCreate(ctx, anchorContact)
	}
	return auth.Require(ctx, "relationship", principal.ActionUpdate)
}

func sameOptionalID(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// writeLinkedInMatchFields lands the ticked boxes through the ordinary contact
// and relationship writers, each with its own audit row and event. It stamps
// the field provenance under the decider who ticked them.
func (s *Store) writeLinkedInMatchFields(ctx context.Context, tx pgx.Tx, in LinkedInMatchApply) (linkedInFieldsWritten, error) {
	var out linkedInFieldsWritten
	f := in.Fields
	if !f.UseAsTitle && !f.UseAsEmployer {
		return out, nil
	}
	// Held already by the apply; asked again here because these writes reach
	// the contact's child rows, which an Art. 17 erasure clears.
	if err := auth.HoldWritableLive(ctx, tx, entityContact, in.ContactID); err != nil {
		return out, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return out, err
	}
	evidence := in.ConnectionID.String()
	stamp := []storekit.FieldStamp{{Field: fieldTitle, EvidenceRef: &evidence}}
	if f.UseAsTitle && f.Position != "" && f.Position != f.ContactTitle {
		contact := ids.ContactID{UUID: in.ContactID}
		if _, err := s.updateContactInTx(ctx, tx, contact, UpdateContactInput{Title: &f.Position}, in.Columns.cols); err != nil {
			return out, err
		}
		if err := storekit.StampFields(ctx, tx, entityContact, in.ContactID, provenance.LinkedInExportSource, capturedBy, stamp); err != nil {
			return out, err
		}
		out.title = true
	}
	if !f.UseAsEmployer {
		return out, nil
	}
	edge, roleWritten, err := s.employFromExport(ctx, tx, in.ContactID, *f.EmployerCompanyID, f.Position)
	if err != nil || edge == ids.Nil {
		return out, err
	}
	// The primary flag is the employer choice itself, so it is stamped on a
	// promoted edge as well as on a created one.
	edgeStamps := []storekit.FieldStamp{{Field: "is_current_primary", EvidenceRef: &evidence}}
	if roleWritten {
		edgeStamps = append(edgeStamps, storekit.FieldStamp{Field: fieldRole, EvidenceRef: &evidence})
	}
	if err := storekit.StampFields(ctx, tx, "relationship", edge, provenance.LinkedInExportSource, capturedBy, edgeStamps); err != nil {
		return out, err
	}
	out.employer = true
	return out, nil
}

// liveEmployment is an employment the contact holds at a company, in the
// slot uq_rel_employment keeps to one.
type liveEmployment struct {
	id         ids.UUID
	role       string
	capturedBy string
}

func liveEmploymentAt(ctx context.Context, tx pgx.Tx, contactID, companyID ids.UUID) (*liveEmployment, error) {
	var live liveEmployment
	err := tx.QueryRow(ctx, `SELECT r.id, coalesce(r.role, ''), r.captured_by FROM relationship r
		 WHERE r.contact_id = $1 AND r.company_id = $2 AND `+employment.LiveSlotSQL("r"),
		contactID, companyID).Scan(&live.id, &live.role, &live.capturedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no employment there is an answer, not a failure
	}
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the contact's employment at the placed company: %w", err)
	}
	return &live, nil
}

// employFromExport makes the placed company the contact's current primary
// employer. It returns the edge that now holds the slot, or ids.Nil when
// nothing changed. An employment the contact already holds there is promoted.
// It takes the position as its role only when it has none, because the card
// never showed that role, and it keeps the edge's author. Otherwise the edge
// is created. Either writer demotes the primary edge it replaces.
func (s *Store) employFromExport(
	ctx context.Context, tx pgx.Tx, contactID, companyID ids.UUID, position string,
) (edge ids.UUID, roleWritten bool, err error) {
	live, err := liveEmploymentAt(ctx, tx, contactID, companyID)
	if err != nil {
		return ids.Nil, false, err
	}
	if live == nil {
		created, err := s.CreateRelationshipTx(ctx, tx, employmentFromExport(contactID, companyID, position))
		return created.ID, err == nil && position != "", err
	}
	// updateRelationshipInTx expects its caller to hold relationship:update.
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		return ids.Nil, false, err
	}
	primary := true
	patch := UpdateRelationshipInput{IsCurrentPrimary: &primary}
	if live.role == "" && position != "" {
		patch.Role = &position
	}
	if _, err := s.updateRelationshipInTx(ctx, tx, live.id, patch, live.capturedBy); err != nil {
		return ids.Nil, false, err
	}
	return live.id, patch.Role != nil, nil
}

// employmentFromExport is the edge "Use as employer" creates: the contact's
// current primary employment at the placed company, with the export's position
// as its role. The create demotes the primary edge it replaces.
func employmentFromExport(contactID, companyID ids.UUID, position string) CreateRelationshipInput {
	primary := true
	contact, company := ids.ContactID{UUID: contactID}, ids.CompanyID{UUID: companyID}
	in := CreateRelationshipInput{
		Kind: employmentKind, ContactID: &contact, CompanyID: &company,
		IsCurrentPrimary: &primary, Source: provenance.LinkedInExportSource,
	}
	if position != "" {
		in.Role = &position
	}
	return in
}
