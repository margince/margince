// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetContactCase reads the case holding a contact. A contact merged away
// answers the case resolved, naming where it went.
func (s *Store) GetContactCase(ctx context.Context, contactID ids.UUID) (crmcontracts.ContactCase, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return crmcontracts.ContactCase{}, err
	}
	var out crmcontracts.ContactCase
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		c, err := readContactCase(ctx, tx, contactID)
		if err != nil {
			return err
		}
		if len(c.Members) == 0 {
			out, err = resolvedContactCase(ctx, tx, contactID)
			return err
		}
		out, err = s.contactCaseView(ctx, tx, c)
		return err
	})
	return out, err
}

// contactCaseView renders a case for its reader: the policy and what this
// reader may do with the case under it.
func (s *Store) contactCaseView(ctx context.Context, tx pgx.Tx, c contactCase) (crmcontracts.ContactCase, error) {
	policy, err := config.StoredApplyTx(ctx, tx, s.bootConfig(), SharedMergeReviews)
	if err != nil {
		return crmcontracts.ContactCase{}, err
	}
	authority, err := readCaseAuthority(ctx, tx, c, policy)
	if err != nil {
		return crmcontracts.ContactCase{}, err
	}
	mode := authority.mergeMode()
	out := crmcontracts.ContactCase{
		AnchorId: openapi_types.UUID(c.Anchor), Revision: c.revision(policy),
		Status: crmcontracts.ContactCaseStatusOpen, Confidence: float32(c.Confidence),
		TooLarge: c.TooLarge, PolicyEnabled: policy, MergeMode: mode,
		Members: make([]crmcontracts.ContactCaseMember, 0, len(c.Members)),
		Edges:   make([]crmcontracts.ContactCaseEdge, 0, len(c.Edges)),
		Owners:  caseOwners(c),
	}
	for _, m := range c.Members {
		out.Members = append(out.Members, crmcontracts.ContactCaseMember{
			Id: openapi_types.UUID(m.ID), Name: m.Name, Email: m.Email, Title: m.Title, CompanyName: m.Company,
			OwnerId: (*openapi_types.UUID)(m.OwnerID), OwnerName: m.OwnerName, Version: m.Version, CreatedAt: m.CreatedAt,
		})
	}
	for _, e := range c.Edges {
		edge := crmcontracts.ContactCaseEdge{
			PairId: openapi_types.UUID(e.ID), LeftId: openapi_types.UUID(e.Left), RightId: openapi_types.UUID(e.Right),
			Confidence: float32(e.Confidence), Status: crmcontracts.ContactCaseEdgeStatus(e.Disposition),
			Evidence: []crmcontracts.DedupeEvidence{}, CanDismiss: authority.mayDismiss(e), CanMerge: authority.mayMergePair(e),
		}
		if len(e.Evidence) > 0 {
			if err := json.Unmarshal(e.Evidence, &edge.Evidence); err != nil {
				return crmcontracts.ContactCase{}, fmt.Errorf("contacts: reading a pair's evidence: %w", err)
			}
		}
		out.Edges = append(out.Edges, edge)
	}
	var reason crmcontracts.ContactCaseReason
	switch {
	case c.TooLarge:
		reason = crmcontracts.ContactCaseReasonTooLarge
	case mode == crmcontracts.ContactCaseMergeModeNone:
		reason = crmcontracts.ContactCaseReasonNoWriteAuthority
	}
	out.CanMerge = reason == ""
	if reason != "" {
		out.Reason = &reason
	}
	return out, nil
}

// caseAuthority is what this reader could decide in a case. It names the
// members they may change and whether the policy lets them act as an owner.
// The decision asks again under its locks.
type caseAuthority struct {
	c        contactCase
	mayAct   bool
	writable map[ids.UUID]bool
	// delegator is the actor when the policy is on; the zero principal owns
	// no member.
	delegator principal.Principal
}

func readCaseAuthority(ctx context.Context, tx pgx.Tx, c contactCase, policy bool) (caseAuthority, error) {
	a := caseAuthority{c: c, writable: map[ids.UUID]bool{}}
	if a.mayAct = auth.Allows(ctx, contactEntity, principal.ActionUpdate); !a.mayAct {
		return a, nil
	}
	for _, m := range c.Members {
		writable, err := auth.WritableBy(ctx, tx, contactEntity, m.ID)
		if err != nil {
			return a, err
		}
		a.writable[m.ID] = writable
	}
	if actor, ok := principal.Actor(ctx); ok && policy {
		a.delegator = actor
	}
	return a, nil
}

func (a caseAuthority) delegate(memberIDs []ids.UUID) bool {
	return !a.delegator.UserID.IsZero() && a.c.actorOwnsAMember(a.delegator, memberIDs)
}

// mergeMode answers how this reader could merge the whole case. That is by
// write authority over every member, by the policy as an owner, or not at all.
func (a caseAuthority) mergeMode() crmcontracts.ContactCaseMergeMode {
	members := memberIDsOf(a.c)
	switch {
	case !a.mayAct:
		return crmcontracts.ContactCaseMergeModeNone
	case !slices.ContainsFunc(members, func(id ids.UUID) bool { return !a.writable[id] }):
		return crmcontracts.ContactCaseMergeModeOrdinary
	case a.delegate(members):
		return crmcontracts.ContactCaseMergeModeDelegated
	}
	return crmcontracts.ContactCaseMergeModeNone
}

// mayDismiss mirrors admitPairDecision for one pair of the case.
func (a caseAuthority) mayDismiss(e caseEdge) bool {
	return a.mayAct && ((a.writable[e.Left] && a.writable[e.Right]) || a.delegate([]ids.UUID{e.Left, e.Right}))
}

// mayMergePair mirrors the pair route's merge, which asks write authority
// over both ends and is never admitted by the policy.
func (a caseAuthority) mayMergePair(e caseEdge) bool {
	return a.mayAct && e.Disposition == dispositionOpen && a.writable[e.Left] && a.writable[e.Right]
}

func memberIDsOf(c contactCase) []ids.UUID {
	out := make([]ids.UUID, 0, len(c.Members))
	for _, m := range c.Members {
		out = append(out, m.ID)
	}
	return out
}

// caseOwners groups the members by their live owners. Each of them sees the
// case in their own Worklist.
func caseOwners(c contactCase) []crmcontracts.ContactCaseOwner {
	out := []crmcontracts.ContactCaseOwner{}
	for _, owner := range c.liveOwners(memberIDsOf(c)) {
		entry := crmcontracts.ContactCaseOwner{UserId: openapi_types.UUID(owner), MemberIds: []openapi_types.UUID{}}
		for _, m := range c.Members {
			if m.OwnerLive && *m.OwnerID == owner {
				entry.MemberIds = append(entry.MemberIds, openapi_types.UUID(m.ID))
				entry.Seat = crmcontracts.ContactCaseOwnerSeat(m.OwnerSeat)
				if m.OwnerName != nil {
					entry.Name = *m.OwnerName
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

// resolvedContactCase answers for a contact in no open case. It is resolved
// when a merge took it into another contact, and not found otherwise.
func resolvedContactCase(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (crmcontracts.ContactCase, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return crmcontracts.ContactCase{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, contactEntity, contactID); err != nil {
		return crmcontracts.ContactCase{}, err
	}
	var survivor *ids.UUID
	var archivedAt *time.Time
	err := tx.QueryRow(ctx, `SELECT merged_into_id, archived_at FROM contact WHERE id = @id`,
		pgx.NamedArgs{"id": contactID}).Scan(&survivor, &archivedAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (survivor == nil || archivedAt == nil)) {
		return crmcontracts.ContactCase{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.ContactCase{}, fmt.Errorf("contacts: reading where the contact went: %w", err)
	}
	reason := crmcontracts.ContactCaseReasonResolved
	out := crmcontracts.ContactCase{
		AnchorId: openapi_types.UUID(contactID), Status: crmcontracts.ContactCaseStatusResolved,
		Members: []crmcontracts.ContactCaseMember{}, Edges: []crmcontracts.ContactCaseEdge{},
		Owners: []crmcontracts.ContactCaseOwner{}, MergeMode: crmcontracts.ContactCaseMergeModeNone,
		Reason: &reason,
	}
	// Where it went is the survivor's to tell: a reader who cannot see it
	// learns only that the case is settled.
	if err := auth.EnsureVisible(ctx, tx, contactEntity, *survivor); errors.Is(err, apperrors.ErrNotFound) {
		return out, nil
	} else if err != nil {
		return crmcontracts.ContactCase{}, err
	}
	resolution := crmcontracts.ContactCaseResolution{SurvivorId: openapi_types.UUID(*survivor), ResolvedAt: *archivedAt}
	var by *ids.UUID
	var byName *string
	var at *time.Time
	err = tx.QueryRow(ctx, `
		SELECT p.disposed_by, u.display_name, p.disposed_at
		  FROM dedupe_candidate p LEFT JOIN app_user u ON u.id = p.disposed_by
		 WHERE p.entity_type = 'contact' AND p.disposition = 'merged'
		   AND @id IN (p.left_contact_id, p.right_contact_id)
		 ORDER BY p.disposed_at DESC NULLS LAST LIMIT 1`, pgx.NamedArgs{"id": contactID}).Scan(&by, &byName, &at)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return crmcontracts.ContactCase{}, fmt.Errorf("contacts: reading who settled the case: %w", err)
	default:
		resolution.ResolvedBy, resolution.ResolvedByName = (*openapi_types.UUID)(by), byName
		if at != nil {
			resolution.ResolvedAt = *at
		}
	}
	out.Resolution = &resolution
	return out, nil
}

// contactCaseCursor is the list's keyset: strongest first, anchor as the tie.
type contactCaseCursor struct {
	Confidence float64  `json:"c"`
	Anchor     ids.UUID `json:"a"`
}

// ContactCaseListInput filters one page of cases.
type ContactCaseListInput struct {
	Scope string // mine (default) | unassigned | all
	// Recipient is whose cases mine means; zero is the caller. Visibility
	// stays the caller's either way.
	Recipient ids.UUID
	Cursor    string
	Limit     int
}

const (
	contactCaseListDefault = 25
	contactCaseListMax     = 50
)

// caseSummary is one component as the list ranks it, before it is rendered,
// and the member its reader is shown it from.
type caseSummary struct {
	component contactComponent
	address   ids.UUID
}

// ListContactCases pages the cases the reader can see in one scope,
// strongest first. The page is rendered from the components it ranked.
func (s *Store) ListContactCases(ctx context.Context, in ContactCaseListInput) ([]crmcontracts.ContactCase, string, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, "", err
	}
	in.Limit = min(max(in.Limit, 0), contactCaseListMax)
	if in.Limit == 0 {
		in.Limit = contactCaseListDefault
	}
	var after *contactCaseCursor
	if in.Cursor != "" {
		cur, err := storekit.DecodeOpaque[contactCaseCursor](in.Cursor)
		if err != nil {
			return nil, "", err
		}
		if cur.Anchor.IsZero() {
			return nil, "", &storekit.MalformedCursorError{}
		}
		after = &cur
	}
	var page []crmcontracts.ContactCase
	var next string
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		ranked, err := rankedContactCases(ctx, tx, in.Scope, in.Recipient)
		if err != nil {
			return err
		}
		if after != nil {
			ranked = slices.DeleteFunc(ranked, func(c caseSummary) bool { return !rankedAfter(c, *after) })
		}
		if len(ranked) > in.Limit {
			ranked = ranked[:in.Limit]
			last := ranked[in.Limit-1]
			if next, err = storekit.EncodeOpaque(contactCaseCursor{Confidence: last.component.confidence, Anchor: last.component.anchor}); err != nil {
				return err
			}
		}
		for _, summary := range ranked {
			c, err := readComponentCase(ctx, tx, summary.component, summary.address)
			if err != nil {
				return err
			}
			view, err := s.contactCaseView(ctx, tx, c)
			if err != nil {
				return err
			}
			page = append(page, view)
		}
		return nil
	})
	return page, next, err
}

// CountContactCases is how many cases ListContactCases would page through in
// the same scope, so a badge and the list under it agree.
func (s *Store) CountContactCases(ctx context.Context, in ContactCaseListInput) (int, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		ranked, err := rankedContactCases(ctx, tx, in.Scope, in.Recipient)
		count = len(ranked)
		return err
	})
	return count, err
}

func rankedAfter(c caseSummary, cur contactCaseCursor) bool {
	k := c.component
	return k.confidence < cur.Confidence || (k.confidence == cur.Confidence && compareUUID(k.anchor, cur.Anchor) > 0)
}
