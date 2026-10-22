// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The inbox in the shape a SECOND door serves it.
//
// The HTTP handlers here were the only caller of the store rows until the
// governed tool surface gained a queue of its own (ADR-0055: a passport is
// governed identically on both doors). These four methods are what the tool
// door composes over, and they exist so the two doors cannot come to describe
// one approval differently: the status a caller is told, the lazy expiry folded
// into it, and the evidence that travels with a proposal are decided ONCE, in
// wire(), for whoever is asking.
//
// They add no authority of their own. Every one of them is the same call the
// handlers make, and the admission, the decide-authority probe and the caps a
// passport may spend are all answered underneath.

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListWire is List in the contract shape.
func (s *Service) ListWire(ctx context.Context, in ListInput) ([]crmcontracts.Approval, storekit.Page, error) {
	rows, page, err := s.List(ctx, in)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	now := s.now()
	out := make([]crmcontracts.Approval, 0, len(rows))
	for _, a := range rows {
		out = append(out, wire(a, now))
	}
	if err := s.readDecisionContext(ctx, out, rows); err != nil {
		return nil, storekit.Page{}, err
	}
	withoutOwnersRecords(ctx, out)
	return out, page, nil
}

// GetWire is Get in the contract shape.
func (s *Service) GetWire(ctx context.Context, id ids.ApprovalID) (crmcontracts.Approval, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return crmcontracts.Approval{}, err
	}
	return s.contextWire(ctx, a)
}

// DecideWire is Decide in the contract shape.
func (s *Service) DecideWire(ctx context.Context, id ids.ApprovalID, approve bool, reason *string) (crmcontracts.Approval, error) {
	a, err := s.Decide(ctx, id, approve, reason)
	if err != nil {
		return crmcontracts.Approval{}, err
	}
	return s.committedWire(ctx, a), nil
}

// DecideEditedWire answers the edited decision with the same context as the inbox.
func (s *Service) DecideEditedWire(ctx context.Context, id ids.ApprovalID, edited json.RawMessage, reason *string) (crmcontracts.Approval, error) {
	a, err := s.DecideEditedFor(ctx, id, edited, reason)
	if err != nil {
		return crmcontracts.Approval{}, err
	}
	return s.committedWire(ctx, a), nil
}

func (s *Service) contextWire(ctx context.Context, a row) (crmcontracts.Approval, error) {
	out := []crmcontracts.Approval{wire(a, s.now())}
	if err := s.readDecisionContext(ctx, out, []row{a}); err != nil {
		return crmcontracts.Approval{}, err
	}
	withoutOwnersRecords(ctx, out)
	return out[0], nil
}

// DecideBundleWire is DecideBundle in the contract shape, each member carrying
// what the decision did to it.
func (s *Service) DecideBundleWire(ctx context.Context, bundleID ids.UUID, approve bool, reason *string) ([]crmcontracts.ApprovalBundleMember, error) {
	members, err := s.DecideBundle(ctx, bundleID, approve, reason)
	if err != nil {
		return nil, err
	}
	now := s.now()
	rows := make([]crmcontracts.Approval, 0, len(members))
	subjects := make([]row, 0, len(members))
	for _, member := range members {
		rows = append(rows, wire(member.Approval, now))
		subjects = append(subjects, member.Approval)
	}
	s.readCommittedContext(ctx, rows, subjects)
	withoutOwnersRecords(ctx, rows)
	out := make([]crmcontracts.ApprovalBundleMember, 0, len(members))
	for index, member := range members {
		out = append(out, crmcontracts.ApprovalBundleMember{
			Approval: rows[index],
			Outcome:  crmcontracts.ApprovalBundleMemberOutcome(member.Outcome),
		})
	}
	return out, nil
}

// A context outage must not turn a committed verdict into an apparent refusal.
func (s *Service) readCommittedContext(ctx context.Context, rows []crmcontracts.Approval, subjects []row) {
	if err := s.readDecisionContext(ctx, rows, subjects); err != nil {
		s.logger().ErrorContext(ctx, "approval decisions committed but context could not be read", "err", err)
		for i, subject := range subjects {
			rows[i] = wire(subject, s.now())
			rows[i].Preview = &crmcontracts.ApprovalPreview{Version: 1, State: "unavailable", Changes: []crmcontracts.ApprovalPreviewChange{}}
		}
	}
}

func (s *Service) committedWire(ctx context.Context, a row) crmcontracts.Approval {
	out := []crmcontracts.Approval{wire(a, s.now())}
	s.readCommittedContext(ctx, out, []row{a})
	withoutOwnersRecords(ctx, out)
	return out[0]
}
