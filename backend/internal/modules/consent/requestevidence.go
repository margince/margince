// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// requestedNegotiation reports whether one of these deals is a negotiation the
// recipient asked for. The deal is open and the recipient a current
// stakeholder. The record shows them asking about that same deal. A deal we created is no permission,
// and a request about one deal does not carry another.
//
// Asking about the deal is one of three things, none bounded by age while the
// deal is open, and none dated in the future:
//   - mail they wrote that a connector captured and whose sender was
//     authenticated, or a captured meeting they took part in
//     (capturedMeetingSQL), filed on the deal. A hand-typed activity is not;
//   - a request recorded by hand that names the deal (about_deal_id), which is
//     also where a rep's confirmation from the review form lands, so it is
//     asked once and reused;
//   - the acquisition request the contact came in with, when it cites the deal.
//
// The two request arms count a request a merge carried over only for its own
// addresses (requestCoversSQL).
func requestedNegotiation(ctx context.Context, tx pgx.Tx, deals []ids.UUID, subject subjectRef) (bool, error) {
	if len(deals) == 0 || subject.Kind != entityContact {
		return false, nil
	}
	entity, err := evidenceEntity(ctx, tx, subject.SendingEntity)
	if err != nil {
		return false, err
	}
	args := []any{subject.Kind, subject.ID, subject.Address, subject.ChannelProvider, subject.ChannelUserID,
		deals, handRecordedRequestKinds, followupRequests.acquisitionKinds,
		time.Now().Add(clockSkewAllowance), time.Now(), entity}
	entityArg := storekit.SQLf("$%d", len(args))
	var found bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM deal d
			  JOIN relationship r ON r.deal_id = d.id
			 WHERE d.id = ANY($6)
			   AND `+liveStakeholderSQL+`
			   AND (
			     EXISTS (
			       SELECT 1
			         FROM activity_link l
			         JOIN activity a ON a.id = l.activity_id
			         JOIN activity_participant p ON p.activity_id = a.id
			        WHERE l.entity_type = 'deal' AND l.deal_id = d.id
			          AND a.archived_at IS NULL
			          AND `+activityEntitySQL("a", entityArg)+`
			          AND ((`+correspondence.AuthenticatedInboundSQL("a")+` AND `+authorIsTheSubject+`)
			               OR (`+capturedMeetingSQL("$10")+` AND `+participantIsTheSubject+`)))
			  OR EXISTS (
			       SELECT 1 FROM consent_qualifying_event q
			        WHERE q.contact_id = $2::uuid
			          AND q.about_deal_id = d.id
			          AND q.kind = ANY($7::text[])
			          AND q.occurred_at <= $9
			          AND q.retracted_at IS NULL
			          AND `+requestCoversSQL("q.recorded_for_addresses", "$3")+`
			          AND `+recordedEntitiesSQL("q.recorded_for_entities", entityArg)+`)
			  OR EXISTS (
			       SELECT 1 FROM contact_acquisition_evidence e
			        WHERE e.contact_id = $2::uuid
			          AND e.source_entity_type = 'deal' AND e.source_entity_id = d.id
			          AND e.kind = ANY($8::text[])
			          AND coalesce(e.occurred_at, e.captured_at) <= $9
			          AND e.request_retracted_at IS NULL
			          AND `+requestCoversSQL("e.recorded_for_addresses", "$3")+`
			          AND `+recordedEntitySQL("e.legal_entity_id", entityArg)+`))
		)`, args...).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("consent: read whether the contact asked about this deal: %w", err)
	}
	return found, nil
}

// handRecordedRequestKinds are the hand-recorded exchanges a deal follow-up
// reads: the contact asking, or the rep meeting them about it.
var handRecordedRequestKinds = []string{KindRequestedBySubject, kindInPerson}

// requestScope names which records count as a request for one kind of message.
type requestScope struct {
	acquisitionKinds []string
	eventKinds       []string
	// quoteOnly keeps only events whose request was for a quote.
	quoteOnly bool
	// perDeal keeps only requests about the deal asked about, or naming no
	// deal. A request about another deal is not this message's request.
	perDeal bool
}

// followupRequests and quoteRequests are the two scopes the validators read.
// Permission given in person, a callback asked for and a form enquiry support a
// follow-up. Only a request for a quote or meeting supports a quote.
var (
	followupRequests = requestScope{
		acquisitionKinds: []string{"requested_quote_or_meeting", "in_person_permission"},
		eventKinds:       []string{KindRequestedBySubject, KindInquiry},
	}
	quoteRequests = requestScope{
		acquisitionKinds: []string{"requested_quote_or_meeting"},
		eventKinds:       []string{KindRequestedBySubject},
		quoteOnly:        true,
		perDeal:          true,
	}
)

// requestOnRecord reports whether the record holds this contact asking to hear
// from us, within scope. That is a request a rep recorded, a form enquiry, or
// acquisition evidence of a request.
//
// A request carried over by a merge covers only the addresses of the record it
// was made on (requestCoversSQL).
//
// No age limit: a request covers the follow-up at the agreed timing. A stop or
// a retraction ends it (RetractQualifyingEvent, and the acquisition request's
// own retraction). occurred_at is caller-stated, so a future date does not count.
func requestOnRecord(ctx context.Context, tx pgx.Tx, subject subjectRef, scope requestScope) (bool, error) {
	_, found, err := latestRequest(ctx, tx, subject, scope, ids.UUID{})
	return found, err
}

// latestRequest is requestOnRecord with the date of the newest request in
// scope, which a quote puts against a marketing objection. A perDeal scope
// reads only requests about deal, or naming no deal; a zero deal reads only
// the ones naming none. So a newer request for one quote does not lift an
// objection for another quote's deal.
func latestRequest(ctx context.Context, tx pgx.Tx, subject subjectRef, scope requestScope, deal ids.UUID) (time.Time, bool, error) {
	if subject.Kind != entityContact {
		// Both tables are keyed on contact_id; a lead holds neither.
		return time.Time{}, false, nil
	}
	entity, err := evidenceEntity(ctx, tx, subject.SendingEntity)
	if err != nil {
		return time.Time{}, false, err
	}
	args := []any{subject.ID, time.Now().Add(clockSkewAllowance),
		scope.eventKinds, scope.acquisitionKinds, scope.quoteOnly, scope.perDeal, zeroUUIDAsNull(deal),
		subject.Address, entity}
	entityArg := storekit.SQLf("$%d", len(args))
	var newest *time.Time
	err = tx.QueryRow(ctx, `
		SELECT max(asked_at) FROM (
			SELECT coalesce(e.occurred_at, e.captured_at) AS asked_at
			  FROM contact_acquisition_evidence e
			 WHERE e.contact_id = $1::uuid
			   AND e.kind = ANY($4::text[])
			   AND coalesce(e.occurred_at, e.captured_at) <= $2
			   AND e.request_retracted_at IS NULL
			   AND (NOT $6 OR e.source_entity_type IS DISTINCT FROM 'deal'
			        OR e.source_entity_id = $7::uuid)
			   AND `+requestCoversSQL("e.recorded_for_addresses", "$8")+`
			   AND `+recordedEntitySQL("e.legal_entity_id", entityArg)+`
			UNION ALL
			SELECT q.occurred_at
			  FROM consent_qualifying_event q
			 WHERE q.contact_id = $1::uuid
			   AND q.kind = ANY($3::text[])
			   AND q.occurred_at <= $2
			   AND q.retracted_at IS NULL
			   AND (q.asks_for_quote OR NOT $5)
			   AND (NOT $6 OR q.about_deal_id IS NULL OR q.about_deal_id = $7::uuid)
			   AND `+requestCoversSQL("q.recorded_for_addresses", "$8")+`
			   AND `+recordedEntitiesSQL("q.recorded_for_entities", entityArg)+`
		) requests`, args...).Scan(&newest)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("consent: read what this contact asked for: %w", err)
	}
	if newest == nil {
		return time.Time{}, false, nil
	}
	return *newest, true, nil
}
