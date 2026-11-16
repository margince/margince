// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the recipient's country asks of a confirm link before it is minted. A
// details check may have to leave out the news question. A consent mail may
// have to answer the subject's own request.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// confirmRules is what the recipient's rules decide about one link.
type confirmRules struct {
	asksNews bool
	// subjectRequest is the request a consent link answers, zero when none is
	// on record.
	subjectRequest ids.UUID
}

// confirmRulesTx decides both for a contact's link. A consent link minted by
// an operator under rules that need a request is refused without one.
func (s *Store) confirmRulesTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, kind, address string, operatorAsks bool,
) (confirmRules, error) {
	out := confirmRules{asksNews: true}
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: entityContact, ID: contactID.String()})
	if err != nil {
		return out, err
	}
	rules, _, found := applicableRules(country.Code)
	if kind == LinkRecordConfirmation {
		out.asksNews = !found || !rules.RecordConfirmationWithoutNews
		return out, nil
	}
	if kind != LinkConsentConfirmation || !operatorAsks {
		return out, nil
	}
	out.subjectRequest, err = s.subscriptionRequestTx(ctx, tx, contactID, address)
	if err != nil {
		return out, err
	}
	if found && rules.ConsentConfirmationNeedsRequest && out.subjectRequest.IsZero() {
		return out, &NoSubjectRequestError{}
	}
	return out, nil
}

// NoSubjectRequestError refuses a consent-confirmation mail a user asked for
// when the recipient's rules need the contact's own request and none is on
// record.
type NoSubjectRequestError struct{}

func (e *NoSubjectRequestError) Error() string { return "consent: " + contactIDKey + ": " + e.reason() }

// FieldFault answers the refusal as a 422 naming the contact.
func (e *NoSubjectRequestError) FieldFault() (field, code, message string) {
	return contactIDKey, "invalid", e.reason()
}

func (e *NoSubjectRequestError) reason() string {
	return "this contact has not asked to subscribe, and their country's rules allow a " +
		"consent-confirmation mail only in answer to their own request. Record the request " +
		"(a form they posted, a mail they wrote, or a call you log) first"
}

// subscriptionRequestTx is the contact's own request a consent mail answers,
// zero when there is none. It asks the send path's qualifying-event readers.
// First comes a request a user or agent recorded, or a booking inquiry whose
// meeting still exists. Then comes a mail the contact wrote, read again from
// the timeline and stamped, so the link and the grant can name it. The mail
// is the installation's own, so the default legal entity is the sender.
func (s *Store) subscriptionRequestTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, address string) (ids.UUID, error) {
	entity, err := storekit.LegalEntityOf(ctx, tx, ids.UUID{})
	if err != nil {
		return ids.UUID{}, err
	}
	sub, _, err := qualifyingSubjectRef(subjectRef{Kind: entityContact, ID: contactID.String()})
	if err != nil {
		return ids.UUID{}, err
	}
	event, found, err := recordedSubjectQualifyingEvent(ctx, tx, sub, time.Time{}, s.now(), &address, entity)
	if err != nil {
		return ids.UUID{}, err
	}
	if !found {
		event, found, err = inboundQualifyingEvent(ctx, tx,
			subjectRef{Kind: entityContact, ID: contactID.String(), Address: address}, time.Time{}, entity)
		if err != nil || !found {
			return ids.UUID{}, err
		}
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return ids.UUID{}, err
		}
		if err := RecordDerivedQualifyingEvent(ctx, tx, contactID.String(), event, by); err != nil {
			return ids.UUID{}, err
		}
	}
	return qualifyingEventRowTx(ctx, tx, contactID, event)
}

// qualifyingEventRowTx names the stored row behind an event: its own id for a
// hand-recorded one, else the row its source record was stamped as.
func qualifyingEventRowTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, event QualifyingEvent) (ids.UUID, error) {
	if event.ID != "" {
		return ids.Parse(event.ID)
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM consent_qualifying_event
		 WHERE contact_id = $1 AND source_entity_type = $2 AND source_entity_id = $3::uuid`,
		contactID, event.SourceEntityType, event.SourceEntityID).Scan(&id)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: naming the request this link answers: %w", err)
	}
	return id, nil
}
