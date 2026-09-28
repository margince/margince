// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The account's contacts, paged.
//
// The 360 carries a contacts SECTION: the top 25 by the same ranking, with a flag
// saying more exist. This is the surface behind that flag — every contact on the
// account, searchable and filterable, in the order the section already used.
//
// ONE ranking, not two. contacts.RankContacts decides the order in both places, so
// the twenty-sixth contact a reader pages to is the twenty-sixth the section
// would have shown had it been longer. A second spelling here would let the
// summary and the list disagree about who matters, on the one screen that shows
// both.

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContactListQuery is one page request over an account's contacts.
type ContactListQuery struct {
	Status *contacts.Engagement
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
// skipping or repeating contacts. It carries the contact id rather than an
// offset so a contact added between pages cannot shift the window.
type contactCursor struct {
	Sort string    `json:"s"`
	ID   ids.UUID  `json:"i"`
	AsOf time.Time `json:"a"`
	// Anchor is the POSITION the previous page ended at, rather than the row
	// that happened to hold it. A contact archived, or renamed out of the
	// search, between two pages is gone from the slice — a resume point that
	// has to be FOUND cannot survive that, while one that can be placed can.
	// Absent on a token minted before this field existed; legacyOffset answers
	// those.
	Anchor *contactAnchor `json:"k,omitempty"`
}

// contactAnchor carries every value the orders here sort by, so the anchor can
// be sorted back into a later page's slice and its place read off.
//
// It holds the engagement INPUTS rather than the engagement: `recommended`
// ranks on EngagementOf, and a token carrying the verdict would freeze a
// derivation that belongs to the contacts module. Carrying what that derivation
// reads leaves one definition of engagement, in the module that owns it.
type contactAnchor struct {
	FullName     string     `json:"n"`
	Strength     int        `json:"g"`
	LastSeen     *time.Time `json:"l"`
	Inbound90d   int        `json:"ib"`
	Outbound90d  int        `json:"ob"`
	LastInbound  *time.Time `json:"li"`
	LastOutbound *time.Time `json:"lo"`
}

// ContactPage lists the account's contacts for one page of the Contacts tab.
//
// The whole visible roster is read and ranked, then sliced: the ranking is a
// function of values folded per contact (engagement, strength) that SQL does not
// hold, so there is no order to push into the query. StrengthForCompanyContacts is
// already the account-sized read the 360 performs, and this shares it rather
// than opening a second one.
func (s *Service) ContactPage(
	ctx context.Context, companyID ids.CompanyID, q ContactListQuery,
) (crmcontracts.CompanyContactListResponse, error) {
	// The same admission AssembleScoped states, for the same reason: this is
	// the paging surface behind that page's contacts section and must not be
	// readable on weaker terms than the section it pages.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	var out crmcontracts.CompanyContactListResponse
	// ONE instant for the whole walk, taken from the cursor when there is one.
	//
	// The ranking is a function of `now` — strength decays and engagement
	// flips — so a page that recomputed it at its own arrival time sorted
	// against a different order than the page before it, and a contact whose
	// standing crossed the boundary in between was served twice or not at all.
	// The page looks complete either way, which is what makes it the same
	// failure the 25-row truncation had, moved to the page boundary.
	//
	// The token already carried this instant and nothing read it. Reading it is
	// also not a new promise: the shared cursor parameter in the contract
	// already claims stability under concurrent updates.
	now, err := s.walkInstant(q.Cursor)
	if err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	// The custom-field catalog opens a transaction of its own, so it is read
	// before this one takes the connection — the same order Graph uses.
	active, err := s.contacts.ActiveCompanyColumns(ctx)
	if err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// The company gate first, and the same one the 360 opens with: an
		// account the caller cannot see must answer not-found here too, or this
		// endpoint becomes the way to discover it.
		if _, err := s.contacts.GetCompanyTx(ctx, tx, companyID, storekit.LiveOnly, active); err != nil {
			return err
		}
		// nil: this endpoint takes no project. It is the account's whole
		// contact list, and narrowing it to a body of work would answer a
		// question nobody asked here.
		all, err := contacts.StrengthForCompanyContacts(ctx, tx, companyID, now, nil)
		if err != nil {
			return err
		}
		rows, err := s.rankedContactRows(ctx, tx, companyID, all, q, now)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	return out, nil
}

// rankedContactRows applies the filters, the order and the page slice, then
// resolves identity for the contacts that survive — identity last, so a name is
// read for the page rather than for the account.
func (s *Service) rankedContactRows(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	all []contacts.ContactStrength, q ContactListQuery, now time.Time,
) (crmcontracts.CompanyContactListResponse, error) {
	// An omitted sort and an explicit `recommended` are the same order, so they
	// must mint the same cursor: two spellings of one order would make a token
	// from either refuse the other, for a difference no caller can see.
	if q.Sort == "" {
		q.Sort = string(crmcontracts.ListCompanyContactsParamsSortRecommended)
	}
	kept := filterByStatus(all, q.Status)
	identity, err := s.matchingIdentity(ctx, tx, companyID, kept, q.Query)
	if err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	if q.Query != nil {
		kept = keepNamed(kept, identity)
	}
	sortContacts(kept, q.Sort, identity)

	limit := storekit.ClampLimit(q.Limit)
	start, err := cursorOffset(kept, identity, q.Cursor, q.Sort)
	if err != nil {
		return crmcontracts.CompanyContactListResponse{}, err
	}
	page := kept[start:min(start+limit, len(kept))]
	hasMore := start+len(page) < len(kept)

	out := crmcontracts.CompanyContactListResponse{
		Data: make([]crmcontracts.CompanyContact, 0, len(page)),
		Page: crmcontracts.PageInfo{HasMore: hasMore},
	}
	for _, c := range page {
		out.Data = append(out.Data, contactRow(c, identity[c.ContactID], now))
	}
	if hasMore && len(page) > 0 {
		// The SAME instant forward, not a fresh one: `now` is the walk's own
		// origin here, so page three resumes from where page one started
		// rather than re-pinning to page two's arrival.
		last := page[len(page)-1]
		token, err := storekit.EncodeOpaque(contactCursor{
			Sort: q.Sort, ID: last.ContactID.UUID, AsOf: now,
			Anchor: anchorOf(last, identity[last.ContactID]),
		})
		if err != nil {
			return crmcontracts.CompanyContactListResponse{}, err
		}
		out.Page.NextCursor = &token
	}
	return out, nil
}

func contactRow(c contacts.ContactStrength, who contactCard, now time.Time) crmcontracts.CompanyContact {
	row := crmcontracts.CompanyContact{
		ContactId:      openapi_types.UUID(c.ContactID.UUID),
		FullName:       who.fullName,
		Title:          who.title,
		Engagement:     crmcontracts.ContactEngagement(contacts.EngagementOf(c.Strength)),
		Strength:       contacts.StrengthToWire(c.Strength, now),
		LastInboundAt:  c.Strength.LastInbound,
		LastOutboundAt: c.Strength.LastOutbound,
	}
	return row
}

func filterByStatus(all []contacts.ContactStrength, want *contacts.Engagement) []contacts.ContactStrength {
	if want == nil {
		return all
	}
	kept := make([]contacts.ContactStrength, 0, len(all))
	for _, c := range all {
		if contacts.EngagementOf(c.Strength) == *want {
			kept = append(kept, c)
		}
	}
	return kept
}

// keepNamed drops the contacts the name search did not match. Identity is only
// read for candidates, so a contact absent from the map did not match.
func keepNamed(all []contacts.ContactStrength, identity map[ids.ContactID]contactCard) []contacts.ContactStrength {
	kept := make([]contacts.ContactStrength, 0, len(all))
	for _, c := range all {
		if _, ok := identity[c.ContactID]; ok {
			kept = append(kept, c)
		}
	}
	return kept
}

// sortContacts orders the page.
//
// `recommended` delegates to contacts.RankContacts — the same call the 360's
// section makes, which is what keeps the summary and this list agreeing. The
// other three are plain column orders, each ending in the contact id so a page
// boundary falls in the same place every time.
func sortContacts(all []contacts.ContactStrength, order string, identity map[ids.ContactID]contactCard) {
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
			return all[i].ContactID.String() < all[j].ContactID.String()
		})
	case "strength":
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].Strength.Strength != all[j].Strength.Strength {
				return (all[i].Strength.Strength > all[j].Strength.Strength) == !ascending
			}
			return all[i].ContactID.String() < all[j].ContactID.String()
		})
	case "name":
		sort.SliceStable(all, func(i, j int) bool {
			ai, bi := identity[all[i].ContactID].fullName, identity[all[j].ContactID].fullName
			if !strings.EqualFold(ai, bi) {
				return (strings.ToLower(ai) < strings.ToLower(bi)) == ascending
			}
			return all[i].ContactID.String() < all[j].ContactID.String()
		})
	default:
		contacts.RankContacts(all)
	}
}

// walkInstant is the moment this page ranks against: the cursor's, continuing a
// walk, or this request's, starting one.
//
// THE TOKEN IS UNSIGNED. storekit.EncodeOpaque is base64 over JSON, and that
// package's own note says a well-formed token is not yet a valid position — the
// caller checks the fields before trusting them. So an instant that names no
// walk is refused rather than honoured: zero, because a token this service
// minted always carries one, and the future, because no page has been served
// from there.
//
// A BACKDATED one is honoured, and deliberately. It re-ranks rows the caller
// can already see, against an order those rows held; that is what resuming an
// old walk means, and refusing it would break the long pause this pinning
// exists to survive.
func (s *Service) walkInstant(token *string) (time.Time, error) {
	now := s.now().UTC()
	if token == nil || *token == "" {
		return now, nil
	}
	pos, err := storekit.DecodeOpaque[contactCursor](*token)
	if err != nil {
		return time.Time{}, err
	}
	if pos.AsOf.IsZero() || pos.AsOf.After(now) {
		return time.Time{}, fmt.Errorf(
			"company360: the cursor names an instant no page was served from: %w",
			&storekit.MalformedCursorError{})
	}
	return pos.AsOf.UTC(), nil
}

// cursorOffset resolves a page token to the index the next page starts at.
//
// The token names where the previous page ended, so this page starts after that
// place — which is a place in the ORDER, not a row. Asking instead which index
// the anchor row sits at makes the answer depend on the anchor still being
// there, and a contact archived between two pages takes the rest of the walk
// with it.
func cursorOffset(
	all []contacts.ContactStrength, identity map[ids.ContactID]contactCard,
	token *string, order string,
) (int, error) {
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
	if pos.Anchor == nil {
		return legacyOffset(all, pos.ID)
	}
	return resumeAfter(all, identity, pos, order), nil
}

// anchorOf records the values every order here sorts by, for the contact a page
// ended on.
func anchorOf(c contacts.ContactStrength, who contactCard) *contactAnchor {
	return &contactAnchor{
		FullName:     who.fullName,
		Strength:     c.Strength.Strength,
		LastSeen:     c.Strength.LastInteraction,
		Inbound90d:   c.Strength.Inbound90d,
		Outbound90d:  c.Strength.Outbound90d,
		LastInbound:  c.Strength.LastInbound,
		LastOutbound: c.Strength.LastOutbound,
	}
}

// resumeAfter is the index the next page starts at: where the anchor belongs in
// this page's slice, whether or not the anchor itself is still in it.
//
// The anchor is sorted back INTO the slice by the same sortContacts the page
// used, rather than compared against it by a second copy of the order. A copy
// would be one more thing to keep in step with RankContacts, and the two
// disagreeing is precisely the skipped contact this exists to prevent.
//
// The anchor sorts equal to its own surviving row and is appended after it, so
// a stable sort leaves it last of the pair: taking the LAST match means the
// anchor's row is behind the resume point rather than served twice.
func resumeAfter(
	all []contacts.ContactStrength, identity map[ids.ContactID]contactCard,
	pos contactCursor, order string,
) int {
	anchorID := ids.From[ids.ContactKind](pos.ID)
	augmented := make([]contacts.ContactStrength, len(all), len(all)+1)
	copy(augmented, all)
	augmented = append(augmented, contacts.ContactStrength{
		ContactID: anchorID,
		Strength: contacts.RelationshipStrength{
			Strength:        pos.Anchor.Strength,
			LastInteraction: pos.Anchor.LastSeen,
			Inbound90d:      pos.Anchor.Inbound90d,
			Outbound90d:     pos.Anchor.Outbound90d,
			LastInbound:     pos.Anchor.LastInbound,
			LastOutbound:    pos.Anchor.LastOutbound,
		},
	})

	// The name the anchor was RANKED under, not the one its row carries now: a
	// contact renamed mid-walk would otherwise resume from a place the previous
	// page never ended at.
	named := make(map[ids.ContactID]contactCard, len(identity)+1)
	maps.Copy(named, identity)
	card := named[anchorID]
	card.fullName = pos.Anchor.FullName
	named[anchorID] = card

	sortContacts(augmented, order, named)

	start := 0
	for i := range augmented {
		if augmented[i].ContactID == anchorID {
			start = i
		}
	}
	return start
}

// legacyOffset resolves a token minted before the cursor carried its anchor's
// position, by finding the anchor row itself.
//
// It keeps the old refusal, because it is the only honest answer left: without
// the anchor's sort values there is nothing to place, and guessing a position
// is how a page silently skips contacts. These tokens stop arriving once the
// pages in flight at the deploy have been walked.
func legacyOffset(all []contacts.ContactStrength, anchor ids.UUID) (int, error) {
	for i, c := range all {
		if c.ContactID.UUID == anchor {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("company360: the cursor names a contact no longer on this account: %w",
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
// hundreds of rows, not the tsvector-sized corpus GET /contacts searches.
func (s *Service) matchingIdentity(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	candidates []contacts.ContactStrength, search *string,
) (map[ids.ContactID]contactCard, error) {
	if len(candidates) == 0 {
		return map[ids.ContactID]contactCard{}, nil
	}
	contactIDs := make([]ids.ContactID, len(candidates))
	for i, c := range candidates {
		contactIDs[i] = c.ContactID
	}
	identity, err := contactIdentity(ctx, tx, companyID, contactIDs)
	if err != nil {
		return nil, err
	}
	if search == nil || strings.TrimSpace(*search) == "" {
		return identity, nil
	}
	needle := strings.ToLower(strings.TrimSpace(*search))
	matched := make(map[ids.ContactID]contactCard, len(identity))
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
