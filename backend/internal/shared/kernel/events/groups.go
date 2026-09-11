// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// The consumer groups: WHO reads the streams, as distinct from which types
// exist and where each one rides, which is the catalog's job.
//
// The two answer different questions and change for different reasons — a new
// event type touches the catalog, a new consuming module touches this file — so
// they are kept apart rather than growing one file that a reader has to scan
// past half of to reach either.

import "sort"

// Group is a §4.3 consumer group: one per consuming module, so each
// module sees every event once and scales horizontally inside the group.
type Group struct {
	Name    string
	Streams []string
}

// Groups returns the consumer groups with their subscribed streams —
// the events.md §4.3 set plus the later consumers; the census test
// mirrors the full list. cg:workflows and cg:read-model subscribe to
// everything by design; cg:audit-stream also does, because its "all
// actor.type=agent events" slice cuts across every stream and Redis
// consumer groups can only partition by stream, not by envelope field —
// the actor filter is in-process, like the workspace filter.
func Groups() []Group {
	all := coreStreams()
	forEntities := func(entities ...string) []string {
		keys := make([]string, len(entities))
		for i, e := range entities {
			keys[i] = StreamPrefix + e
		}
		sort.Strings(keys)
		return keys
	}
	return []Group{
		{Name: "cg:context-graph", Streams: forEntities(contactStreamEntity, companyStreamEntity, dealStreamEntity, activityStreamEntity, leadStreamEntity)},
		// The interaction-edge projection (CG-DDL-1 / ADR-0078). Its OWN group
		// rather than a second handler on cg:context-graph: a projection
		// rebuild must not be able to stall embedding freshness, and the two
		// have unrelated failure modes.
		{Name: "cg:graph-edge", Streams: forEntities(activityStreamEntity, contactStreamEntity)},
		// The audience-change corrector: when a human LIMITS a message after
		// the derived models were built, the derived signals citing it narrow
		// and the thread's scan watermark drops so the next extraction pass
		// re-reads under the new audience. Its own group rather than a second
		// handler on cg:graph-edge for the same isolation reason: re-scoping
		// signals must not be able to stall the edge projection, and vice
		// versa.
		{Name: "cg:audience-rescope", Streams: forEntities(activityStreamEntity)},
		// The LinkedIn ghost matcher (ADR-0078 §8b). Its own group rather than
		// a second handler on cg:graph-edge: that consumer lives in the search
		// module and this call belongs to contacts, and a module never reaches
		// into a sibling. It listens on the contact and company streams —
		// a contact appearing is a chance to attach a ghost, and so is an
		// account appearing, because employer resolution is what most ghosts
		// are waiting on.
		{Name: "cg:linkedin-match", Streams: forEntities(contactStreamEntity, companyStreamEntity)},

		// Its own group rather than a second handler on cg:contact-auto-enrich:
		// this repair attaches mail the workspace already holds to a record it
		// already has, and must keep flowing when an enrichment that calls out
		// to a model is slow or wedged.
		{Name: "cg:cohort-promote", Streams: forEntities(contactStreamEntity)},
		// Turning a won deal into what its partner earned. Its own group
		// because accrual is money: a projection rebuild or an embedding
		// backlog must never be able to stall it, and a failure to accrue must
		// never look like a failure to index. It listens on the deal stream,
		// where deal.stage_changed carries both the win and the reopen that
		// reverses one.
		{Name: "cg:commissions", Streams: forEntities(dealStreamEntity)},
		// Turning records that already settle something into stage evidence: a
		// contract turning active, a buyer confirming a room version, a held
		// meeting. Both streams, because contract.* and deal_room.* ride the
		// deal one and activity.captured rides its own.
		//
		// Its own group rather than a handler on cg:commissions: that consumer
		// accrues money and must never be stalled behind an evidence write,
		// and evidence must keep flowing when an accrual is wedged. The two
		// share a stream and nothing else.
		{Name: "cg:stage-evidence", Streams: forEntities(dealStreamEntity, activityStreamEntity)},
		// How each proposed stage move was received, counted on the ledger the
		// launch gate reads. The APPROVAL stream, because a verdict rides
		// there — including `expired`, the window closing on a card nobody
		// answered, which is a real outcome written by the sweep rather than by
		// any human.
		//
		// Its own group rather than a handler on cg:stage-evidence: that
		// consumer WRITES the evidence a proposal rests on and this one counts
		// what happened to the proposal, so a wedged extraction must not stop
		// the measurement that says whether the feature may stay on.
		{Name: "cg:stage-progression-outcome", Streams: forEntities(approvalStreamEntity)},
		// Closing an introduction the contact answered. Its own group because
		// the evidence is perishable in one direction: `replied` may only be
		// reached from a captured message, so a lane wedged behind an
		// enrichment backlog leaves every introduction reading as unanswered,
		// which is what a refusal looks like to the rep who asked.
		//
		// BOTH streams. activity.captured is the reply arriving; the contact
		// stream is the repair, because capture promotes an address to a
		// contact in a transaction AFTER the one that wrote the message. A
		// message captured before its sender was a contact names nobody the
		// activity arm can act on, and without the contact event that reply
		// would be lost permanently while the ask read unanswered.
		{Name: "cg:intro-advance", Streams: forEntities(activityStreamEntity, contactStreamEntity)},
		// What the installation owes a contact it obtained without asking them.
		// Contact stream only: the duty is decided from how the contact was
		// acquired, and the acquisition row is written in the same transaction
		// as the contact and the event that announces it.
		{Name: "cg:notice-case-open", Streams: forEntities(contactStreamEntity)},
		// What happened in a Deal Room, written onto the deal's timeline. Its own
		// group because a room's traffic is live and conversational while the
		// projections above are batchy: a backlog of embeddings must not delay the
		// note that says the buyer just asked something. It listens on the deal
		// stream, where every deal_room event rides.
		{Name: "cg:deal-room-timeline", Streams: forEntities(dealStreamEntity)},
		// The AI-activity projection (ai_task_run): what the rail and the
		// activity feed read. Its own group because a projection backlog must
		// not be able to stall anything that spends money or moves a record,
		// and because it is the only group on the aitask stream — an
		// all-stream group would carry this traffic for nothing.
		{Name: "cg:ai-activity", Streams: forEntities(aiTaskStreamEntity)},
		// Filling a contact from what their employer's site already published
		// (ADR-0072 arc). Its own group for the same reason as the matcher
		// above: the deep read fills a published contact DURING the crawl, so
		// a contact who arrives afterwards is never matched against what that
		// site said. It listens on the contact stream alone — the fill is
		// keyed on the contact, and an account appearing enriches nobody
		// until a contact is filed against it, which is itself a contact event.
		{Name: "cg:contact-auto-enrich", Streams: forEntities(contactStreamEntity)},
		// The prompt half of captured-company auto-enrich (ADR-0072
		// arc): a company appearing or changing queues the workspace's
		// enrich pass NOW instead of leaving a company created between two
		// daily sweeps without a dossier for up to a day. Its own group
		// rather than a second handler on cg:linkedin-match for the standing
		// reason: that consumer belongs to search-adjacent matching, this one
		// to capture enrichment, and the two must not share a cursor. It
		// listens on the company stream alone — the pass it queues
		// re-derives which companies are due from the database, so no
		// other entity's event can make one due that this stream's events do
		// not already announce.
		{Name: "cg:company-auto-enrich", Streams: forEntities(companyStreamEntity)},
		// Mail landing queues the signature-enrich pass, so a contact who wrote
		// this morning is read now rather than tonight. Its own group rather
		// than a second handler on an existing activity consumer: this one
		// queues a MODEL-backed pass, and a group whose retries spend the
		// customer's token budget must not share a cursor with a projection
		// that is cheap to replay.
		//
		// The CONTACT stream too, because the pass selects a contact joined to
		// their own open mail and either half can be what was missing. A sender
		// nobody had classified has no contact while their mail lands; the
		// counterparty verdict mints them minutes later, and contact.created is
		// the only notice that their first mail — the one carrying the signature
		// block — has become readable.
		{Name: "cg:capture-enrich", Streams: forEntities(activityStreamEntity, contactStreamEntity)},
		// A card attached to captured mail imports itself. Its own group beside
		// the one above rather than a second handler on it, because the two do
		// not fail alike: that one queues a MODEL-backed pass and this one
		// PARSES a file, so a deployment with no model runs this and not that,
		// and one cursor between them would tie the deterministic half to the
		// budgeted half's retries.
		{Name: "cg:vcard-ingest", Streams: forEntities(activityStreamEntity)},
		// Automatic enrichment from a LICENSED provider (ADR-0101/PI-EVT-1).
		// Its own group rather than a second handler on the pass above,
		// because the two differ in what a failure costs: that one reads a
		// page the workspace already crawled, this one SPENDS the customer's
		// credits, and a consumer whose retries buy data must not share a
		// cursor with one whose retries are free.
		{Name: "cg:contact-data", Streams: forEntities(contactStreamEntity)},
		{Name: "cg:overnight-agent", Streams: forEntities(activityStreamEntity, dealStreamEntity, leadStreamEntity, approvalStreamEntity)},
		{Name: "cg:workflows", Streams: all},
		{Name: "cg:capture", Streams: forEntities(captureStreamEntity)},
		{Name: "cg:flow-bridge", Streams: forEntities(contactStreamEntity, dealStreamEntity, activityStreamEntity)},
		{Name: "cg:read-model", Streams: all},
		{Name: "cg:audit-stream", Streams: all},
		// The outbound-webhook fan-out (E10/S-E10.6): a subscription may
		// name any published event type, so this group listens on every
		// stream and matches per-subscription event_types in-process.
		{Name: "cg:webhooks", Streams: all},
	}
}
