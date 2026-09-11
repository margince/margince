// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package contacts owns the contact, company and lead aggregates —
// creation, dedupe, keyset listing, optimistic updates, archive, the
// two-record merge (features/01 §1.3) and lead promotion (§6.4) — as
// store + contract mapping + transport handlers + the contacts slice of
// the datasource provider, flat per ADR-0054 §3.
//
// Tables owned: contact, contact_email, contact_phone, contact_consent,
// contact_profile_field, company, company_domain, relationship,
// partner, lead, lead_score_history, lead_manual_signal, lead_source,
// lead_disqualify_reason, contact_signature_enrich_state,
// contact_provider_claim, provider_applied_field, company_vat_check,
// relationship_nudge_dismissal, sdr_handoff, sdr_handoff_event,
// sdr_handoff_reason.
// The sdr_handoff trio is a prospect being passed from an SDR to an account
// executive: the row carrying its current state, the append-only transitions
// behind it, and the administered reason list a refusal names. It lives here
// because the SUBJECT is a lead or a contact; the deal an acceptance produces is
// an outcome, and a module never imports a sibling — compose wires the caller
// that creates a deal and accepts a handoff in one act. Not the delivery
// briefing the agent surface calls a handoff, which is about a project already
// sold.
// company_vat_check is what the EU register answered about a company's
// VAT ID and the consultation number proving we asked: the profile field
// holds the number a page stated, this holds whether it is real.
// lead_source and lead_disqualify_reason are the two administered lead
// vocabularies: the pick lists behind "where did this come from" and "why
// was it dropped", and the weighting the scorer reads.
// relationship_nudge_dismissal is a lapsed contact ONE reader has set aside,
// and it is its own table rather than a disposition because a nudge is not an
// activity: the decay lane's row carries the contact's id, so there is no
// activity to hang the judgement on. Never permanent — the moment it lifts is
// NOT NULL, so a dismissal that silently deleted somebody from a rep's
// attention has no value to store.
// lead_score_history is the retained series behind "Explain This Score"
// (ADR-0105): the breakdown is written with the score and read back
// verbatim, because a decomposition recomputed at read time explains a
// number the record no longer carries. lead_manual_signal is what a rep
// knows and capture cannot fetch.
// contact_provider_claim is what a licensed data provider asserted about
// one of our contacts (ADR-0101): the domain owns the VALUES because it
// decides what a claim means and how it renders, while integrations owns
// the run that bought it.
// Merge and promotion additionally relink rows in deal, activity_link,
// list_member, taggable, consent_event and provider_run inside their
// single transaction — the ratified cross-aggregate ownership call of the
// primary aggregate; nothing else in this module WRITES a sibling table.
// The provider_run relink is the merge's alone: it decides which of two
// colliding LIVE runs keeps the one-live-run index, which is knowable
// only where both records' run states are visible at once.
// The contact list READS two of collections' — tag and taggable — for the
// contract's `tag` filter: a tagged contact is a link row whichever module
// writes it, and the alternative is a declared filter answered by nobody.
//
// Imports shared + platform + the generated contract only; never a
// sibling module. Every write rides storekit's audit+outbox shape and
// every entry point is gated by platform/auth.
package contacts
