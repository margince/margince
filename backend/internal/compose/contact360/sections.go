// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The per-section reads. Each one carries its own object grant so a
// caller missing it loses that section and keeps the page.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The strength number is narrowed with the rest of the page. A page scoped to
// one body of work that scored the relationship across every project would put
// the right timeline under a number computed somewhere else, and cite
// contributing activity ids the reader cannot see on the page.
func (s *Service) strengthSection(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	now time.Time, within *ids.ProjectID, out *crmcontracts.Contact360,
) error {
	rs, err := s.contacts.ContactStrengthTx(ctx, tx, contactID, now, within)
	if err != nil {
		return err
	}
	wire := contacts.StrengthToWire(rs, now)
	out.Strength = &wire
	return nil
}

// relationshipChangesSection reports what happened to the relationship, as
// opposed to what it currently is.
//
// It sits beside the strength section rather than inside it because the two
// answer different questions and a caller may hold the grant for one reading
// and still lose the other to a section fault. Both fold the same §4 curve, so
// they cannot disagree about the same window.
func (s *Service) relationshipChangesSection(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	now time.Time, within *ids.ProjectID, out *crmcontracts.Contact360,
) error {
	changes, err := s.contacts.ContactRelationshipChangesTx(ctx, tx, contactID, now, within)
	if err != nil {
		return err
	}
	wire := make([]crmcontracts.ContactRelationshipChange, 0, len(changes))
	for _, c := range changes {
		item := crmcontracts.ContactRelationshipChange{
			Kind: crmcontracts.ContactRelationshipChangeKind(c.Kind),
			At:   c.At,
		}
		// Days and the two bands are per-kind, so each is set only where it
		// means something. A zero "days" on a band move would read as "this
		// happened today".
		if c.Days > 0 {
			days := c.Days
			item.Days = &days
		}
		if c.FromBucket != "" {
			from := crmcontracts.ContactRelationshipChangeFromBucket(c.FromBucket)
			to := crmcontracts.ContactRelationshipChangeToBucket(c.ToBucket)
			item.FromBucket, item.ToBucket = &from, &to
		}
		wire = append(wire, item)
	}
	out.RelationshipChanges = &wire
	return nil
}

// employmentsSection lists this contact's employment edges, current primary
// first — the header's "who they work for" and the career ribbon's history
// come from the same rows, so a former employer is never overwritten.
func (s *Service) employmentsSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *crmcontracts.Contact360) error {
	if err := requireRead(ctx, "relationship"); err != nil {
		return err
	}
	limit := sectionCap
	kind := "employment"
	rows, page, err := s.contacts.ListRelationshipsTx(ctx, tx, contacts.ListRelationshipsInput{
		Kind: &kind, ContactID: &contactID, Limit: &limit,
	})
	if err != nil {
		return err
	}
	data := make([]crmcontracts.Contact360Employment, 0, len(rows))
	for _, r := range rows {
		if r.CompanyID == nil {
			continue // an employment edge with no employer names nothing
		}
		e := crmcontracts.Contact360Employment{
			RelationshipId:   openapi_types.UUID(r.ID),
			CompanyId:        openapi_types.UUID(r.CompanyID.UUID),
			IsCurrentPrimary: r.IsCurrentPrimary,
			Role:             r.Role,
			StartedAt:        r.StartedAt,
			EndedAt:          r.EndedAt,
		}
		name, err := s.companyName(ctx, tx, *r.CompanyID)
		if err != nil {
			return err
		}
		if name != "" {
			e.CompanyName = &name
		}
		data = append(data, e)
	}
	// Current primary first, then the rest as the store ordered them: the
	// header reads the first row, so the employer they hold today must not
	// depend on insertion order.
	for i := range data {
		if data[i].IsCurrentPrimary && i != 0 {
			data[0], data[i] = data[i], data[0]
			break
		}
	}
	out.Employments = &struct {
		Data []crmcontracts.Contact360Employment `json:"data"`
		Page crmcontracts.PageInfo               `json:"page"`
		// The store's own answer, not len(rows) >= cap: ListRelationshipsTx
		// over-fetches by one to know this exactly, and guessing it would
		// report has_more on a contact holding exactly 25 employments and send
		// the client to a page that does not exist.
	}{Data: data, Page: crmcontracts.PageInfo{HasMore: page.HasMore}}
	return nil
}

// companyName resolves an employer's display name. A name the caller
// cannot read is simply absent — the edge still shows, without asserting a
// company the reader has no grant for.
func (s *Service) companyName(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (string, error) {
	// The two refusals the paragraph above promises, spelled as the two
	// questions they actually are. The row-scope miss was the only one the
	// statement asked — its id-and-archived_at predicate says nothing about
	// who is reading — so a caller holding no company grant at all read
	// employer names through the employment edge. auth.Require answers the
	// object question and auth.EnsureVisible the row one, and both refuse by
	// leaving the name absent rather than failing the section, because an
	// employment the reader may see is still a true edge.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "", nil
		}
		return "", err
	}
	if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return "", nil
		}
		return "", err
	}
	var name string
	err := tx.QueryRow(ctx, `SELECT display_name FROM company WHERE id = $1 AND archived_at IS NULL`, companyID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		// Archived, or outside this caller's row scope. The edge still shows;
		// it just does not assert a company name the reader has no grant for.
		// This is the ONLY tolerated outcome — any other error has already
		// aborted the transaction, and continuing past it makes the NEXT
		// section fail with an error that names the wrong cause.
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read employer name: %w", err)
	}
	return name, nil
}

// dealRolesSection lists the stakeholder seats this contact holds. The role
// is what the edge records; it is never inferred from a job title.
func (s *Service) dealRolesSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *crmcontracts.Contact360) error {
	if err := requireRead(ctx, "relationship"); err != nil {
		return err
	}
	if err := requireRead(ctx, "deal"); err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactPos := arg(contactID)
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return err
	}
	if dealScope == "" {
		dealScope = "true"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.deal_id, r.role, d.name, s.name
		FROM relationship r
		JOIN deal d ON d.id = r.deal_id AND d.archived_at IS NULL
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE r.kind = 'deal_stakeholder' AND r.contact_id = $%d
		  AND r.archived_at IS NULL AND (%s)
		ORDER BY r.id
		LIMIT %d`, contactPos, dealScope, sectionCap+1), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	data := make([]crmcontracts.Contact360DealRole, 0, sectionCap)
	for rows.Next() {
		var dr crmcontracts.Contact360DealRole
		var relID, dealID ids.UUID
		var role *string
		if err := rows.Scan(&relID, &dealID, &role, &dr.DealTitle, &dr.DealStage); err != nil {
			return err
		}
		dr.RelationshipId = openapi_types.UUID(relID)
		dr.DealId = openapi_types.UUID(dealID)
		if role != nil {
			dr.Role = *role
		}
		data = append(data, dr)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	hasMore := len(data) > sectionCap
	if hasMore {
		data = data[:sectionCap]
	}
	out.DealRoles = &struct {
		Data []crmcontracts.Contact360DealRole `json:"data"`
		Page crmcontracts.PageInfo             `json:"page"`
	}{Data: data, Page: crmcontracts.PageInfo{HasMore: hasMore}}
	return nil
}
