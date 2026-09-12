// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read's auto-enrich lane (CAP-PARAM-7, ADR-0072/A118): a read the
// captured-company sweep triggered applies its findings DIRECTLY instead
// of staging a confirm-first proposal — the system chose to enrich the company,
// so there is no human to confirm. The company fields + facts land through the same
// fill-empty + human-precedence machinery a human accept uses (so a human value
// is never overwritten and a re-run after a worker death is idempotent); site
// contacts still stage as leads (strangers stay staged, NEVER-8). The sweep cursor
// records the terminal outcome for observability, never gating the read.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// systemAutoEnrichActor is the requested-by sentinel the auto-enrich sweep
// stamps on the deep reads it triggers. A read carrying it takes the auto-apply
// lane below; every other read stages as before.
const systemAutoEnrichActor = "system:capture_auto_enrich"

// The terminal outcomes the auto-apply lane records on the sweep cursor
// (capture_auto_enrich_state.last_outcome — its CHECK carries the same set).
const (
	autoEnrichOutcomeApplied = "applied"
	autoEnrichOutcomeEmpty   = "empty"
	autoEnrichOutcomeFailed  = "failed"
)

// fillMatchedContacts fills the site's role and details onto the contacts this
// workspace ALREADY records at the company, and answers the strangers — the
// ones the act still has to ASK about.
//
// The match is deliberately narrow (exact email, or exactly one confident name
// among the company's own employees); everyone else, and every ambiguity, is a
// stranger and stages. A fill that FAILS must not cost the lead either: the
// contact falls through to staging so they still reach a human, and the reason
// goes to the log.
//
// It runs before the staging transaction rather than inside it, because these
// writes and those proposals answer for different things — an apply that fails
// on one contact must not roll back the questions asked about the others.
func (w *siteDeepReadWorker) fillMatchedContacts(ctx context.Context, companyID ids.CompanyID, found []siteContact) []siteContact {
	strangers := make([]siteContact, 0, len(found))
	for _, contact := range found {
		matched, err := w.contacts.ApplySiteContactFields(ctx, companyID, contacts.SiteContactFields{
			Name:            contact.Name,
			Role:            contact.Role,
			PublishedEmail:  contact.PublishedEmail,
			LinkedinURL:     contact.LinkedinURL,
			EvidenceSnippet: contact.EvidenceSnippet,
			SourceURL:       contact.SourceURL,
		})
		if err != nil {
			// The lead survives a failed fill — the contact still reaches a human.
			// Stated here rather than borrowed: the store answers no match on
			// error today, but that is its contract to keep, and this loop's
			// invariant should not depend on remembering it.
			matched = false
			w.log.WarnContext(ctx, "auto-enrich: filling a matched site contact failed",
				"company", companyID.String(), "contact", contact.Name, "err", err)
		}
		if matched {
			continue
		}
		strangers = append(strangers, contact)
	}
	return strangers
}

// isAutoEnrichRequest reports whether a deep read was triggered by the
// auto-enrich sweep rather than a human.
func isAutoEnrichRequest(requestedBy string) bool { return requestedBy == systemAutoEnrichActor }

// applyForRequester is the human lane's terminal step: the requester asked for
// this read, so its company fields and facts land directly rather than becoming a
// proposal that asks them to confirm what they just requested.
//
// It is deliberately NOT autoApply: that one also moves the auto-enrich sweep
// cursor, which is bookkeeping for reads nobody asked for. A human read has no
// cursor to advance, and advancing one would tell the sweep it had already
// enriched a company it never looked at.
//
// Site contacts still stage as leads, on the same NEVER-8 grounds the automatic
// lane keeps them: a contact the site published is a new record about a human
// being, not a column on the company the requester named.
func (w *siteDeepReadWorker) applyForRequester(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, mergedFields []evidencedField, mergedFacts []contacts.DeepReadFact, mergedContacts []siteContact) ([]ids.UUID, error) {
	companyID := ids.From[ids.CompanyKind](*claim.CompanyID)
	// Staged first for the reason autoApply states: the leads were evidenced
	// independently of the company columns, so an apply failure must not drop them.
	proposalIDs, err := w.stageSiteLeads(ctx, args.SiteReadID, claim, w.fillMatchedContacts(ctx, companyID, mergedContacts), ids.NewV7())
	if err != nil {
		return nil, err
	}
	fields := deepReadFields(mergedFields)
	if len(fields) == 0 && len(mergedFacts) == 0 {
		return proposalIDs, nil
	}
	if err := w.contacts.ApplyDeepRead(ctx, contacts.DeepReadProposal{
		CompanyID:  companyID,
		SourceURL:  claim.SeedURL,
		SiteReadID: args.SiteReadID,
		Fields:     fields,
		Facts:      mergedFacts,
	}); err != nil {
		// The contacts are staged; surfacing the error finishes the read failed,
		// which is what tells the human the company half did not land.
		return proposalIDs, fmt.Errorf("applying the deep read: %w", err)
	}
	return proposalIDs, nil
}

// autoApply is the auto-enrich lane's terminal step: apply the company fields +
// facts directly, stage site contacts as leads, and record the cursor outcome.
func (w *siteDeepReadWorker) autoApply(ctx context.Context, args SiteDeepReadArgs, claim contacts.SiteReadClaim, mergedFields []evidencedField, mergedFacts []contacts.DeepReadFact, mergedContacts []siteContact) ([]ids.UUID, error) {
	companyID := ids.From[ids.CompanyKind](*claim.CompanyID)

	// Site contacts stage as leads regardless of the field/fact apply outcome —
	// the leads were evidenced independently of the company columns, and dropping
	// them on an apply failure would break the strangers-stay-staged invariant
	// (NEVER-8). Staged first so a later apply error cannot skip them.
	//
	// One act, one bundle, staged in one transaction: the company's own fields and
	// facts are APPLIED on this lane rather than proposed, so the leads are
	// everything this act asked about, and they reach the inbox together.
	proposalIDs, err := w.stageSiteLeads(ctx, args.SiteReadID, claim, w.fillMatchedContacts(ctx, companyID, mergedContacts), ids.NewV7())
	if err != nil {
		return nil, err
	}

	fields := deepReadFields(mergedFields)
	outcome := autoEnrichOutcomeEmpty
	var applyErr error
	if len(fields) > 0 || len(mergedFacts) > 0 {
		if err := w.contacts.ApplyDeepRead(ctx, contacts.DeepReadProposal{
			CompanyID:  companyID,
			SourceURL:  claim.SeedURL,
			SiteReadID: args.SiteReadID,
			Fields:     fields,
			Facts:      mergedFacts,
		}); err != nil {
			applyErr, outcome = err, autoEnrichOutcomeFailed
		} else {
			outcome = autoEnrichOutcomeApplied
		}
	}
	if err := w.autoEnrich.MarkResolved(ctx, companyID, outcome); err != nil {
		// A missed terminal write at worst lets the next sweep reconsider the
		// company, which the dossier-exists gate then filters out (or, on a failed
		// apply, retries it) — never the read's success or failure.
		w.log.WarnContext(ctx, "auto-enrich cursor not recorded", "company", companyID.String(), "outcome", outcome, "err", err)
	}
	if applyErr != nil {
		// The contacts are staged; surface the apply failure so the read finishes
		// failed and the sweep retries the company.
		return proposalIDs, fmt.Errorf("auto-applying the deep read: %w", applyErr)
	}
	return proposalIDs, nil
}
