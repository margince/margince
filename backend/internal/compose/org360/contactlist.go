// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The account's people, paged.
//
// The 360 carries a people SECTION: the top 25 by the same ranking, with a flag
// saying more exist. This is the surface behind that flag — every contact on the
// account, searchable and filterable, in the order the section already used.
//
// ONE ranking, not two. people.RankContacts decides the order in both places, so
// the twenty-sixth contact a reader pages to is the twenty-sixth the section
// would have shown had it been longer. A second spelling here would let the
// summary and the list disagree about who matters, on the one screen that shows
// both.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContactListQuery is one page request over an account's contacts.
type ContactListQuery struct {
	Status *people.Engagement
	Query  *string
	Sort   string
	Cursor *string
	Limit  *int
}

// contactCursor is the position a page resumes from.
//
// It carries the SORT it was minted under, because every order here is derived
// in Go from values the database cannot sort by — a token replayed against a
// different order would resume from a position that order never had, silently
// skipping or repeating contacts. It carries the person id rather than an
// offset so a contact added between pages cannot shift the window.
type contactCursor struct {
	Sort string    `json:"s"`
	ID   ids.UUID  `json:"i"`
	AsOf time.Time `json:"a"`
}

// ContactPage lists the account's contacts for one page of the People tab.
//
// The whole visible roster is read and ranked, then sliced: the ranking is a
// function of values folded per contact (engagement, strength) that SQL does not
// hold, so there is no order to push into the query. StrengthForOrgContacts is
// already the account-sized read the 360 performs, and this shares it rather
// than opening a second one.
func (s *Service) ContactPage(
	ctx context.Context, orgID ids.OrganizationID, q ContactListQuery,
) (crmcontracts.OrganizationContactListResponse, error) {
	var out crmcontracts.OrganizationContactListResponse
	now := s.now().UTC()
	// The custom-field catalog opens a transaction of its own, so it is read
	// before this one takes the connection — the same order Graph uses.
	active, err := s.people.ActiveOrganizationColumns(ctx)
	if err != nil {
		return crmcontracts.OrganizationContactListResponse{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// The organization gate first, and the same one the 360 opens with: an
		// account the caller cannot see must answer not-found here too, or this
		// endpoint becomes the way to discover it.
		if _, err := s.people.GetOrganizationTx(ctx, tx, orgID, storekit.LiveOnly, active); err != nil {
			return err
		}
		all, err := people.StrengthForOrgContacts(ctx, tx, orgID, now)
		if err != nil {
			return err
		}
		rows, err := s.rankedContactRows(ctx, tx, orgID, all, q, now)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return crmcontracts.OrganizationContactListResponse{}, err
	}
	return out, nil
}

// rankedContactRows applies the filters, the order and the page slice, then
// resolves identity for the contacts that survive — identity last, so a name is
// read for the page rather than for the account.
func (s *Service) rankedContactRows(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	all []people.ContactStrength, q ContactListQuery, now time.Time,
) (crmcontracts.OrganizationContactListResponse, error) {
	// An omitted sort and an explicit `recommended` are the same order, so they
	// must mint the same cursor: two spellings of one order would make a token
	// from either refuse the other, for a difference no caller can see.
	if q.Sort == "" {
		q.Sort = string(crmcontracts.Recommended)
	}
	kept := filterByStatus(all, q.Status)
	identity, err := s.matchingIdentity(ctx, tx, orgID, kept, q.Query)
	if err != nil {
		return crmcontracts.OrganizationContactListResponse{}, err
	}
	if q.Query != nil {
		kept = keepNamed(kept, identity)
	}
	sortContacts(kept, q.Sort, identity)

	limit := storekit.ClampLimit(q.Limit)
	start, err := cursorOffset(kept, q.Cursor, q.Sort)
	if err != nil {
		return crmcontracts.OrganizationContactListResponse{}, err
	}
	page := kept[start:min(start+limit, len(kept))]
	hasMore := start+len(page) < len(kept)

	out := crmcontracts.OrganizationContactListResponse{
		Data: make([]crmcontracts.OrganizationContact, 0, len(page)),
		Page: crmcontracts.PageInfo{HasMore: hasMore},
	}
	for _, c := range page {
		out.Data = append(out.Data, contactRow(c, identity[c.PersonID], now))
	}
	if hasMore && len(page) > 0 {
		token, err := storekit.EncodeOpaque(contactCursor{
			Sort: q.Sort, ID: page[len(page)-1].PersonID.UUID, AsOf: now,
		})
		if err != nil {
			return crmcontracts.OrganizationContactListResponse{}, err
		}
		out.Page.NextCursor = &token
	}
	return out, nil
}

func contactRow(c people.ContactStrength, who contactCard, now time.Time) crmcontracts.OrganizationContact {
	row := crmcontracts.OrganizationContact{
		PersonId:       openapi_types.UUID(c.PersonID.UUID),
		FullName:       who.fullName,
		Title:          who.title,
		Engagement:     crmcontracts.ContactEngagement(people.EngagementOf(c.Strength)),
		Strength:       people.StrengthToWire(c.Strength, now),
		LastInboundAt:  c.Strength.LastInbound,
		LastOutboundAt: c.Strength.LastOutbound,
	}
	return row
}

func filterByStatus(all []people.ContactStrength, want *people.Engagement) []people.ContactStrength {
	if want == nil {
		return all
	}
	kept := make([]people.ContactStrength, 0, len(all))
	for _, c := range all {
		if people.EngagementOf(c.Strength) == *want {
			kept = append(kept, c)
		}
	}
	return kept
}

// keepNamed drops the contacts the name search did not match. Identity is only
// read for candidates, so a contact absent from the map did not match.
func keepNamed(all []people.ContactStrength, identity map[ids.PersonID]contactCard) []people.ContactStrength {
	kept := make([]people.ContactStrength, 0, len(all))
	for _, c := range all {
		if _, ok := identity[c.PersonID]; ok {
			kept = append(kept, c)
		}
	}
	return kept
}

// sortContacts orders the page.
//
// `recommended` delegates to people.RankContacts — the same call the 360's
// section makes, which is what keeps the summary and this list agreeing. The
// other three are plain column orders, each ending in the person id so a page
// boundary falls in the same place every time.
func sortContacts(all []people.ContactStrength, order string, identity map[ids.PersonID]contactCard) {
	// Each field in both directions, because a table header is a toggle: the
	// reader who presses "Last exchange" twice is asking for the reverse, and
	// the design system spells that by prefixing a minus onto the column's own
	// field name.
	ascending := !strings.HasPrefix(order, "-")
	switch strings.TrimPrefix(order, "-") {
	case "last_interaction":
		sort.SliceStable(all, func(i, j int) bool {
			a, b := all[i].Strength.LastInteraction, all[j].Strength.LastInteraction
			if (a == nil) != (b == nil) {
				// A contact nobody has ever spoken to sorts last rather than
				// first: a nil date is the absence of an interaction, not one
				// that happened at the zero time.
				return b == nil
			}
			if a != nil && !a.Equal(*b) {
				return a.After(*b) == !ascending
			}
			return all[i].PersonID.String() < all[j].PersonID.String()
		})
	case "strength":
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].Strength.Strength != all[j].Strength.Strength {
				return (all[i].Strength.Strength > all[j].Strength.Strength) == !ascending
			}
			return all[i].PersonID.String() < all[j].PersonID.String()
		})
	case "name":
		sort.SliceStable(all, func(i, j int) bool {
			ai, bi := identity[all[i].PersonID].fullName, identity[all[j].PersonID].fullName
			if !strings.EqualFold(ai, bi) {
				return (strings.ToLower(ai) < strings.ToLower(bi)) == ascending
			}
			return all[i].PersonID.String() < all[j].PersonID.String()
		})
	default:
		people.RankContacts(all)
	}
}

// cursorOffset resolves a page token to a position in the ranked slice.
//
// The token names the last contact of the previous page, so the next one starts
// after it. A contact that has since left the account is not in the slice any
// more: rather than guess a position, the read refuses, because resuming from a
// position that no longer exists is how a page silently skips people.
func cursorOffset(all []people.ContactStrength, token *string, order string) (int, error) {
	if token == nil || *token == "" {
		return 0, nil
	}
	pos, err := storekit.DecodeOpaque[contactCursor](*token)
	if err != nil {
		return 0, err
	}
	if pos.Sort != order {
		return 0, &storekit.CursorSortMismatchError{}
	}
	for i, c := range all {
		if c.PersonID.UUID == pos.ID {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("org360: the cursor names a contact no longer on this account: %w",
		&storekit.MalformedCursorError{})
}

// matchingIdentity reads name and title for the candidate contacts, keeping only
// those whose name or title matches the search when one was given.
//
// The match is done in Go over the rows already in hand rather than as a SQL
// predicate, because the candidate set is the account roster this read has
// already loaded — pushing the filter down would mean a second pass over the
// same table to remove rows we are holding. Case-insensitive substring, which is
// what a reader typing three letters of a surname expects; the account roster is
// hundreds of rows, not the tsvector-sized corpus GET /people searches.
func (s *Service) matchingIdentity(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	candidates []people.ContactStrength, search *string,
) (map[ids.PersonID]contactCard, error) {
	if len(candidates) == 0 {
		return map[ids.PersonID]contactCard{}, nil
	}
	personIDs := make([]ids.PersonID, len(candidates))
	for i, c := range candidates {
		personIDs[i] = c.PersonID
	}
	identity, err := contactIdentity(ctx, tx, orgID, personIDs)
	if err != nil {
		return nil, err
	}
	if search == nil || strings.TrimSpace(*search) == "" {
		return identity, nil
	}
	needle := strings.ToLower(strings.TrimSpace(*search))
	matched := make(map[ids.PersonID]contactCard, len(identity))
	for id, who := range identity {
		title := ""
		if who.title != nil {
			title = *who.title
		}
		if strings.Contains(strings.ToLower(who.fullName), needle) ||
			strings.Contains(strings.ToLower(title), needle) {
			matched[id] = who
		}
	}
	return matched, nil
}
