// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The contacts section: who works at this account, how warm each
// relationship is, what each one does on the account's deals, and whether
// they may be contacted for each purpose.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactsSection lists the account's current employees with their §4
// strength, their stakeholder roles on this account's deals, and their
// per-purpose consent state. Contacts outside the caller's row scope are
// absent — the batch strength read that produced `all` applies that
// predicate itself, which is also why the scores arrive rather than being
// recomputed here: the account roll-up is folded from the same slice.
func contactsSection(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time,
	all []contacts.ContactStrength,
) ([]crmcontracts.Company360Contact, crmcontracts.PageInfo, error) {
	// Ranked BEFORE the cut, because the cut is what the reader sees. `all`
	// arrives ordered by contact id — the roster read's own deterministic order,
	// which is arbitrary as a reading order — and truncating that keeps the
	// first 25 ids rather than the 25 contacts worth looking at. On an account
	// with a hundred employees the one who answered last week sits wherever
	// their id falls, so the section that is supposed to open the page could
	// omit them entirely while reporting nothing worse than has_more.
	//
	// Ranking a COPY: `all` is the same slice the account roll-up folds, and
	// reordering it under that reader would make the summary depend on which
	// section ran first.
	ranked := make([]contacts.ContactStrength, len(all))
	copy(ranked, all)
	contacts.RankContacts(ranked)
	strengths, page := truncate(ranked)
	if len(strengths) == 0 {
		return []crmcontracts.Company360Contact{}, page, nil
	}

	contactIDs := make([]ids.ContactID, len(strengths))
	for i, s := range strengths {
		contactIDs[i] = s.ContactID
	}
	identity, err := contactIdentity(ctx, tx, companyID, contactIDs)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, err
	}
	roles, err := contactDealRoles(ctx, tx, companyID, contactIDs)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, err
	}
	consent, err := contactConsent(ctx, tx, contactIDs)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, err
	}
	// Who on our side can reach each of them. Read for the whole set in one
	// query rather than per contact — see contactroutes.go.
	rawIDs := make([]ids.UUID, len(contactIDs))
	for i, id := range contactIDs {
		rawIDs[i] = id.UUID
	}
	allowed, err := mayReadRoutes(ctx)
	if err != nil {
		return nil, crmcontracts.PageInfo{}, err
	}
	var routes map[ids.UUID]crmcontracts.Company360ContactRoutes
	if allowed {
		routes, err = contactRoutes(ctx, tx, rawIDs, now)
		if err != nil {
			return nil, crmcontracts.PageInfo{}, err
		}
	}

	out := make([]crmcontracts.Company360Contact, 0, len(strengths))
	for _, s := range strengths {
		id := s.ContactID
		card := crmcontracts.Company360Contact{
			ContactId: openapi_types.UUID(id.UUID),
			Strength:  contacts.StrengthToWire(s.Strength, now),
			DealRoles: roles[id],
			Consent:   consent[id],
		}
		if card.DealRoles == nil {
			card.DealRoles = []crmcontracts.Company360DealRole{}
		}
		if card.Consent == nil {
			card.Consent = map[string]crmcontracts.Company360ContactConsent{}
		}
		if route, ok := routes[id.UUID]; ok {
			card.Routes = &route
		}
		if who, ok := identity[id]; ok {
			card.FullName = who.fullName
			card.Title = who.title
			card.PrimaryEmail = who.primaryEmail
			card.ProviderTitle = who.providerTitle
			card.TitleSource = titleSource(who)
		}
		out = append(out, card)
	}
	return out, page, nil
}

// titleSource says which title the roster is showing, so the page can mark a
// purchased one as purchased. Null where there is no title at all: a contact
// nobody has a role for is a gap, not a source.
func titleSource(who contactCard) *crmcontracts.Company360ContactTitleSource {
	switch {
	case who.title != nil && *who.title != "":
		source := crmcontracts.Company360ContactTitleSourceCanonical
		return &source
	case who.providerTitle != nil:
		source := crmcontracts.Company360ContactTitleSourceProvider
		return &source
	default:
		return nil
	}
}

// contactCard is the display identity of one contact.
type contactCard struct {
	fullName     string
	title        *string
	primaryEmail *string
	// providerTitle is a PURCHASED job title, shown only where the canonical
	// one is empty (PO-EXT-9). It fills a blank; it never overwrites or
	// seconds a title a human typed.
	providerTitle *string
}

// contactIdentity reads name, title and the primary email address for a
// contact set. The address arrives through a correlated subquery so a
// contact with none on file still appears: the strength read already
// decided who is on this list, and a join could only shorten it.
func contactIdentity(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, contactIDs []ids.ContactID) (map[ids.ContactID]contactCard, error) {
	// The purchased title rides the same correlated-subquery shape as the
	// address, under two conditions.
	//
	// It is read ONLY where the canonical title is empty: a bought title
	// beside one a human typed would be a second opinion, which PO-EXT-9
	// forbids — what the installation knows wins, and the purchase fills a
	// gap.
	//
	// And only where the claim is about THIS company. A purchased employment
	// claim names its own employer, and a contact whose newest claim says
	// "VP Sales, Globex" must not have "VP Sales" rendered on Acme's roster:
	// that is a false employment assertion on an account page, which is the
	// harm the precedence rule exists to prevent. Matched on the claim's own
	// company_domain against this company's domains, or on its
	// company_name against the display name.
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name, p.title,
		       (SELECT e.email FROM contact_email e
		         WHERE e.contact_id = p.id AND e.archived_at IS NULL`+
		contacts.ReachableEmailOrder+`
		         LIMIT 1),
		       CASE WHEN coalesce(p.title, '') <> '' THEN NULL ELSE
		         (SELECT NULLIF(c.value_json->>'job_title', '')
		            FROM contact_provider_claim c
		           WHERE c.contact_id = p.id AND c.claim_key = 'current_employment'
		             AND (
		               EXISTS (SELECT 1 FROM company_domain d
		                        WHERE d.company_id = $2 AND d.archived_at IS NULL
		                          AND lower(d.domain) = lower(c.value_json->>'company_domain'))
		               OR EXISTS (SELECT 1 FROM company o
		                           WHERE o.id = $2
		                             AND lower(o.display_name) = lower(c.value_json->>'company_name'))
		             )
		           ORDER BY c.retrieved_at DESC
		           LIMIT 1)
		       END
		FROM contact p WHERE p.id = ANY($1)`, contactIDs, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[ids.ContactID]contactCard, len(contactIDs))
	for rows.Next() {
		var id ids.ContactID
		var card contactCard
		if err := rows.Scan(&id, &card.fullName, &card.title, &card.primaryEmail, &card.providerTitle); err != nil {
			return nil, err
		}
		out[id] = card
	}
	return out, rows.Err()
}

// contactDealRoles reads each contact's stakeholder roles on THIS
// account's deals, pruned to the deals the caller can see: a rep who
// cannot read a colleague's deal must not learn who its champion is.
//
// A seat is also an EDGE, so it needs the edge grant as well as the deal's —
// the pair is the fact relationship.read governs. A caller refused it gets an
// empty map rather than an error, because `deal_roles` is required on every
// contact card: contactsSection normalises a missing entry to `[]`, so the
// roster still renders and the roles simply are not there. There is no
// withheld channel per contact to name it in, which is why the refusal is
// swallowed HERE rather than failing the section around it.
func contactDealRoles(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, contactIDs []ids.ContactID) (map[ids.ContactID][]crmcontracts.Company360DealRole, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos, companyPos := arg(contactIDs), arg(companyID)
	edgeBound, err := edgeScope(ctx, arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return map[ids.ContactID][]crmcontracts.Company360DealRole{}, nil
	}
	if err != nil {
		return nil, err
	}
	dealScope, err := scopeClause(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.contact_id, r.deal_id, r.role
		FROM relationship r
		JOIN deal d ON d.id = r.deal_id
		WHERE r.kind = 'deal_stakeholder' AND r.contact_id = ANY($%d)
		  AND r.archived_at IS NULL AND r.ended_at IS NULL
		  AND d.company_id = $%d AND d.archived_at IS NULL
		  AND (%s) AND (%s)
		ORDER BY r.contact_id, r.deal_id`, contactsPos, companyPos, edgeBound, dealScope), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.ContactID][]crmcontracts.Company360DealRole{}
	for rows.Next() {
		var contactID ids.ContactID
		var dealID ids.UUID
		var role *string
		if err := rows.Scan(&contactID, &dealID, &role); err != nil {
			return nil, err
		}
		named := ""
		if role != nil {
			named = *role
		}
		out[contactID] = append(out[contactID], crmcontracts.Company360DealRole{DealId: openapi_types.UUID(dealID), Role: named})
	}
	return out, rows.Err()
}

// contactConsent reads each contact's state per consent purpose. Every
// live purpose appears for every contact: a purpose with no stored row is
// "unknown", which is default-deny for outbound, and leaving the key out
// would let a caller read absence as permission.
func contactConsent(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID) (map[ids.ContactID]map[string]crmcontracts.Company360ContactConsent, error) {
	purposes, err := livePurposeKeys(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ContactID]map[string]crmcontracts.Company360ContactConsent, len(contactIDs))
	for _, id := range contactIDs {
		states := make(map[string]crmcontracts.Company360ContactConsent, len(purposes))
		for _, key := range purposes {
			states[key] = crmcontracts.Company360ContactConsentUnknown
		}
		out[id] = states
	}
	rows, err := tx.Query(ctx, `
		SELECT pc.contact_id, cp.key, pc.state
		FROM contact_consent pc
		JOIN consent_purpose cp ON cp.id = pc.purpose_id AND cp.archived_at IS NULL
		WHERE pc.contact_id = ANY($1)`, contactIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.ContactID
		var key, state string
		if err := rows.Scan(&contactID, &key, &state); err != nil {
			return nil, err
		}
		if states, ok := out[contactID]; ok {
			states[key] = crmcontracts.Company360ContactConsent(state)
		}
	}
	return out, rows.Err()
}

func livePurposeKeys(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT key FROM consent_purpose WHERE archived_at IS NULL ORDER BY key`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var key string
		err := row.Scan(&key)
		return key, err
	})
}
