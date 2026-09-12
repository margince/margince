// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package contactresearch runs the deep-research surface on a contact
// (ADR-0096 Decision 4, UC-E04-02 as amended).
//
// NOTHING IS WRITTEN BEFORE AN EXPLICIT SAVE. A run stages: it returns claims
// and touches no record. The record changes only when a human reviews the
// staged set and accepts specific claims, and the accepted ones land in
// contact_profile_field through the audited write shape — the same table the
// signature-enrichment pass fills, whose evidence_snippet and source_ref are
// both NOT NULL, so a claim that lost its quote or its source cannot be stored
// at all.
//
// The zero-write guarantee is STRUCTURAL, not a rule this package remembers:
// running a research pass needs no store with a write method, so the service
// that runs one does not hold one. Only Save does.
//
// With no provider registered the surface answers "not connected" and stops.
// That is a named state, not an error to retry: retrying changes nothing, and
// a spinner over an absent provider is a lie about what the product is doing.
package contactresearch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/contactdata"
)

// Assembler reads the contact as their reader would see them. Research needs
// only enough to identify the subject to a provider, and it takes that from
// the caller's own gated read rather than a second query.
type Assembler interface {
	Assemble(ctx context.Context, contactID ids.ContactID) (crmcontracts.Contact360, error)
}

// Service runs research and saves what a human accepted.
type Service struct {
	contacts *contacts.Store
	view     Assembler
	provider *contactdata.Registry
	now      func() time.Time
}

// NewService binds the surface to its provider registry. A nil provider inside
// the registry is the supported no-provider configuration.
func NewService(store *contacts.Store, view Assembler, provider *contactdata.Registry, now func() time.Time) *Service {
	return &Service{contacts: store, view: view, provider: provider, now: now}
}

// Run asks the provider what is publicly known about this contact.
//
// It writes nothing. The result is the staged set a human reviews.
func (s *Service) Run(ctx context.Context, contactID ids.ContactID) (crmcontracts.ContactResearchRun, error) {
	// Research is a reading aid for a contact, and asking a third party about a
	// named human is not something an unattended agent does on its own.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ContactResearchRun{}, err
	}
	// The gate that matters runs HERE, in the caller's own composite read: a
	// contact they cannot open cannot be researched, and the refusal comes
	// before any provider learns the name.
	view, err := s.view.Assemble(ctx, contactID)
	if err != nil {
		return crmcontracts.ContactResearchRun{}, err
	}

	out := crmcontracts.ContactResearchRun{
		ContactId:   openapi_types.UUID(contactID.UUID),
		State:       crmcontracts.ContactResearchRunStateNotConnected,
		GeneratedAt: s.now().UTC(),
		Claims:      []crmcontracts.ContactResearchClaim{},
	}
	if !s.provider.Connected() {
		// The honest empty state. Nothing was asked, nothing was read, and the
		// drawer says so rather than showing an empty result that looks like a
		// provider found nothing.
		return out, nil
	}

	result, err := s.provider.Research(ctx, subjectFrom(view))
	if err != nil {
		if errors.Is(err, contactdata.ErrNoProvider) {
			return out, nil
		}
		return crmcontracts.ContactResearchRun{}, fmt.Errorf("run contact research: %w", err)
	}
	out.State = crmcontracts.ContactResearchRunStateReady
	out.SourcesRead = ptr(result.SourcesRead)
	out.Claims = wireClaims(result.Claims)
	out.ProviderName = ptr(s.provider.ProviderName())
	return out, nil
}

// subjectFrom is the ONLY thing a provider learns: who we are asking about.
//
// Not our correspondence, not the deal, not what they said to us. The narrow
// shape is the point — sending relationship context to a third party to get
// public facts back would be the egress this whole port exists to bound.
func subjectFrom(view crmcontracts.Contact360) contactdata.Subject {
	subject := contactdata.Subject{FullName: view.Contact.FullName}
	if view.Contact.Title != nil {
		subject.Title = *view.Contact.Title
	}
	if view.Employments != nil && len(view.Employments.Data) > 0 {
		first := view.Employments.Data[0]
		if first.IsCurrentPrimary && first.CompanyName != nil {
			subject.Employer = *first.CompanyName
		}
	}
	return subject
}

// wireClaims renders the staged claims, dropping any the reader could not
// check. A claim with no openable source is not evidence, and showing it
// beside ones that are would teach a reader to trust the wrong things.
func wireClaims(claims []contactdata.Claim) []crmcontracts.ContactResearchClaim {
	out := make([]crmcontracts.ContactResearchClaim, 0, len(claims))
	for _, claim := range claims {
		sources := make([]crmcontracts.ContactResearchSource, 0, len(claim.Sources))
		for _, source := range claim.Sources {
			if !webURL(source.URL) {
				// A provider's URL is untrusted. Only http(s) leaves this
				// service: a javascript: or data: source would be a payload
				// the client renders as a link, and refusing it here means
				// every consumer is safe rather than each one remembering.
				continue
			}
			sources = append(sources, crmcontracts.ContactResearchSource{
				Label: source.Label,
				Url:   source.URL,
				Quote: ptr(source.Quote),
			})
		}
		if len(sources) == 0 {
			continue
		}
		out = append(out, crmcontracts.ContactResearchClaim{
			// Numbered by what the reader SEES. Numbering by the provider's
			// index leaves gaps where a claim was dropped, and a conversation
			// angle citing "Claim 3" would point at nothing.
			Ordinal:    len(out) + 1,
			Body:       claim.Body,
			Confidence: crmcontracts.ContactResearchClaimConfidence(claim.Confidence),
			Sources:    sources,
		})
	}
	return out
}

// Save writes the claims a human accepted, and only those.
//
// This is the first and only write in the whole surface. It lands in
// contact_profile_field, where the two evidence columns are NOT NULL — so the
// "checkable or refused" rule is the schema's, not this function's good
// manners. A saved research claim is a FACT about who the contact is, not a
// conversation claim: it has no status, no needs-review and no dismissal,
// because nobody said it in a conversation we captured.
func (s *Service) Save(ctx context.Context, contactID ids.ContactID, in crmcontracts.SaveContactResearchRequest) (int, error) {
	// Ahead of the empty-list return: the operation is human-only regardless
	// of what it would have written, and an agent seat posting an empty list
	// must see the same refusal a non-empty one gets. Nothing lands from
	// either path, but "human-only" is a property of the call, not of its
	// payload.
	if err := auth.RequireHuman(ctx); err != nil {
		return 0, err
	}
	if len(in.Claims) == 0 {
		// Saving nothing is not an error — a reader who dismissed every claim
		// made a decision, and it is the one this surface exists to allow.
		return 0, nil
	}
	return s.contacts.SaveResearchClaims(ctx, contactID, toClaimInputs(in.Claims))
}

// toClaimInputs carries each accepted claim with the source that makes it
// checkable. The quote and the URL both ride along: a saved claim a reader
// cannot trace back to a document is exactly what the review step existed to
// stop, so the write refuses one that lost them.
func toClaimInputs(claims []crmcontracts.SaveContactResearchClaim) []contacts.ResearchClaimInput {
	out := make([]contacts.ResearchClaimInput, 0, len(claims))
	for _, claim := range claims {
		out = append(out, contacts.ResearchClaimInput{
			Field:     string(claim.Field),
			Value:     claim.Value,
			Quote:     claim.SourceQuote,
			SourceURL: claim.SourceUrl,
		})
	}
	return out
}

// webURL admits only the two schemes a citation can honestly carry.
func webURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}

func ptr[T any](v T) *T { return &v }
