// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Groups of contacts that may be duplicates. Contact pairs reach the queue as
// these cases, never one pair at a time. Each live owner of a member sees the
// case in their own queue.

import (
	"context"
	"errors"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sourceContactCase is the lane's source; the row id is the case's anchor.
const sourceContactCase = "contact_case"

// The store's case scopes, as the contacts list spells them.
const (
	CaseScopeMine       = "mine"
	CaseScopeUnassigned = "unassigned"
	CaseScopeAll        = "all"
)

// CaseQuery asks for the cases one recipient holds: under mine, those with a
// member the recipient owns. The store binds the recipient before it ranks,
// so a capped page never drops the recipient's case for someone else's.
type CaseQuery struct {
	Scope     string
	Recipient ids.UUID
	Limit     int
}

// ContactCases is the contacts module's case read, under the reader's scope.
type ContactCases interface {
	OpenCases(ctx context.Context, q CaseQuery) ([]ContactCase, error)
	CountCases(ctx context.Context, q CaseQuery) (int, error)
}

// ContactCase is one case as the lane draws it.
type ContactCase struct {
	Anchor     ids.UUID
	Confidence float64
	// Owners are the members' live owners, in id order.
	Owners []ids.UUID
	Face   crmcontracts.AttentionContactCase
}

// WithContactCases binds the case read.
func WithContactCases(c ContactCases) Option {
	return func(s *Service) { s.contactCases = c }
}

// caseQuery maps this read's scope onto the store's, naming whose cases a
// personal page holds.
func (s *Service) caseQuery(ctx context.Context, depth int) (CaseQuery, error) {
	switch s.taskScope {
	case TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			return CaseQuery{}, apperrors.ErrPermissionDenied
		}
		return CaseQuery{Scope: CaseScopeMine, Recipient: actor.UserID, Limit: depth}, nil
	case TasksOwnedBy:
		return CaseQuery{Scope: CaseScopeMine, Recipient: s.taskOwner, Limit: depth}, nil
	case TasksUnassigned:
		return CaseQuery{Scope: CaseScopeUnassigned, Limit: depth}, nil
	}
	return CaseQuery{Scope: CaseScopeAll, Limit: depth}, nil
}

// openContactCases reads the cases this page is for and how many there are,
// in one scope.
func (s *Service) openContactCases(ctx context.Context, depth int) ([]crmcontracts.AttentionItem, int, error) {
	if s.contactCases == nil {
		return nil, 0, nil
	}
	q, err := s.caseQuery(ctx, depth)
	if err != nil {
		return nil, 0, err
	}
	cases, err := s.contactCases.OpenCases(ctx, q)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// A reader who may not read contacts has no case to review.
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	count, err := s.contactCases.CountCases(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(cases))
	for _, c := range cases {
		items = append(items, caseItem(c, s.caseRecipient(q, c, readerOf(ctx))))
	}
	return items, count, nil
}

// caseRecipient is the owner the row is raised for. A personal page names its
// own recipient. A wider page names the reader when they own a member, then a
// teammate, then the first owner; a case nobody owns names nobody.
func (s *Service) caseRecipient(q CaseQuery, c ContactCase, reader ids.UUID) *ids.UUID {
	switch {
	case q.Scope == CaseScopeMine:
		return &q.Recipient
	case len(c.Owners) == 0:
		return nil
	case slices.Contains(c.Owners, reader):
		return &reader
	}
	for _, owner := range c.Owners {
		if slices.Contains(s.noticeOwners, owner) {
			return &owner
		}
	}
	return &c.Owners[0]
}

// caseItem renders one case. Like a pair it carries no title: the client
// words it in the reader's language.
func caseItem(c ContactCase, recipient *ids.UUID) crmcontracts.AttentionItem {
	kind := subjectContact
	confidence := float32(c.Confidence)
	face := c.Face
	item := crmcontracts.AttentionItem{
		Id:          c.Anchor.String(),
		Source:      crmcontracts.AttentionItemSource(sourceContactCase),
		Kind:        &kind,
		Confidence:  &confidence,
		Actions:     []crmcontracts.AttentionItemActions{},
		ContactCase: &face,
	}
	if recipient != nil {
		item.RequestedDeciderId = idPtr(*recipient)
	}
	return item
}
