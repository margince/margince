// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft

// The service: two gated reads, one grounded write, no transaction.
//
// There is no pool here and no store with a write method, which is the
// zero-write guarantee stated as a dependency rather than as a rule — the same
// shape persondraft uses, for the same reason. A contributor who wanted this
// endpoint to persist something would have to add a field to this struct.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/compose/persondraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LeadReader is the caller's own read of the lead. Gated inside the store
// (auth.Require plus auth.EnsureVisible), so a lead this caller may not open
// refuses before a word is written.
type LeadReader interface {
	GetLead(ctx context.Context, id ids.LeadID, archived storekit.ArchivedFilter) (crmcontracts.Lead, error)
}

// ActivityLister is the caller's own read of what has been said to this lead,
// newest first. Same gate as the lead page's own timeline, so a draft can only
// stand on messages this caller could open themselves.
//
// Narrowed to the one question rather than taking the activities store's whole
// list input: this package may ask for ONE lead's correspondence and nothing
// else, and an interface carrying the filter struct would let it ask for
// anything. compose binds the seam (compose/leaddraftseam.go).
type ActivityLister interface {
	ForLead(ctx context.Context, id ids.LeadID) ([]crmcontracts.Activity, error)
}

// Request is the transport's body, narrowed to what the writer needs.
//
// One field, and no recipient among them: the lead in the path IS the
// recipient, which is what makes this the same shape persondraft writes for and
// not a second kind of request.
type Request struct {
	// Intent is the caller's own steering ("shorter", "ask for Tuesday"). The
	// one input they typed, and the one input outside the fence.
	Intent string
}

// Service writes one draft per call.
type Service struct {
	leads    LeadReader
	acts     ActivityLister
	lane     persondraft.Completer
	envelope *draftfloor.Resolver
	// voice reads the sender's own Voice DNA. The READ seam, not the voice
	// store: this package promises to write nothing, and a store handed in
	// whole would carry the learning-signal writes with it.
	voice draftvoice.Reader
	log   *slog.Logger
}

// NewService binds the draft to the reads it is grounded in and the model lane
// that writes it. lane may be nil: that is a deployment running no model, and
// persondraft's deterministic floor is the answer.
func NewService(leads LeadReader, acts ActivityLister, lane persondraft.Completer) *Service {
	return &Service{leads: leads, acts: acts, lane: lane, envelope: draftfloor.NewResolver()}
}

// WithEnvelope replaces the resolver that answers what language to write in,
// what time it is and who is signing.
func (s *Service) WithEnvelope(resolver *draftfloor.Resolver) *Service {
	if resolver != nil {
		s.envelope = resolver
	}
	return s
}

// WithVoice binds the sender's voice profile read, so a rep who has built one
// gets their own writing here too.
func (s *Service) WithVoice(reader draftvoice.Reader, log *slog.Logger) *Service {
	s.voice = reader
	s.log = log
	return s
}

// Draft writes one email. It performs no write of any kind.
func (s *Service) Draft(
	ctx context.Context, leadID ids.LeadID, req Request,
) (crmcontracts.AccountEmailDraft, error) {
	// Human-only: drafting spends the workspace's model budget on prose for a
	// person to send under their own name.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	// The gate that matters runs HERE, in the caller's own read: a lead they
	// cannot see refuses before a word is written.
	//
	// LiveOnly, so a terminal lead is a 404 rather than a draft. Both closures
	// archive the row — disqualified, and promoted to a contact — and neither
	// is a record to open a new conversation from: the promoted one's
	// correspondence belongs to the person it became.
	lead, err := s.leads.GetLead(ctx, leadID, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	// A draft addressed to nobody is not a message. Refused before the model
	// call rather than after it, so a lead with no address costs nothing.
	if lead.Email == nil || string(*lead.Email) == "" {
		return crmcontracts.AccountEmailDraft{}, httperr.Validation("email", "missing",
			"this lead has no email address on record, so there is nobody to write to")
	}
	activities, err := s.acts.ForLead(ctx, leadID)
	if err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	envelope := s.envelope.Resolve(ctx,
		persondraft.CorrespondenceTextOf(activities),
		ConversationState(activities, s.envelope.Now()))
	in := FromLead(lead, activities, req.Intent, envelope)
	// Loaded after the lead read, so a caller who may not see this lead is
	// refused before their voice profile is touched at all.
	voice := draftvoice.Load(ctx, s.voice, s.log)
	draft, by, err := persondraft.Write(ctx, s.lane, in, voice)
	if err != nil {
		return crmcontracts.AccountEmailDraft{}, err
	}
	return persondraft.Wire(draft, by, voice.Degraded), nil
}
