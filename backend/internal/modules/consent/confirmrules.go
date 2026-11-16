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
	var asked bool
	out.subjectRequest, asked, err = s.subscriptionRequestTx(ctx, tx, contactID, address)
	if err != nil {
		return out, err
	}
	if found && rules.ConsentConfirmationNeedsRequest && !asked {
		return out, &NoSubjectRequestError{}
	}
	return out, nil
}

// codeInvalid is the field code of a 422 that names a value the rules refuse.
const codeInvalid = "invalid"

// NoSubjectRequestError refuses a consent-confirmation mail a user asked for
// when the recipient's rules need the contact's own request and none is on
// record.
type NoSubjectRequestError struct{}

func (e *NoSubjectRequestError) Error() string { return "consent: " + contactIDKey + ": " + e.reason() }

// FieldFault answers the refusal as a 422 naming the contact.
func (e *NoSubjectRequestError) FieldFault() (field, code, message string) {
	return contactIDKey, codeInvalid, e.reason()
}

func (e *NoSubjectRequestError) reason() string {
	return "this contact has not asked to subscribe, and their country's rules allow a " +
		"consent-confirmation mail only in answer to their own request. Record the request " +
		"(a form they posted, a mail they wrote, or a call you log) first"
}

// subscriptionRequestTx is the contact's own request a consent mail answers.
// It is the request owner's answer for subscriptionRequests (newestRequest),
// else a mail the contact wrote, read again from the timeline and stamped. The
// mail is the installation's own, so the default legal entity is the sender.
// found is false when there is none; request is zero for acquisition evidence,
// which has no qualifying-event row to name.
func (s *Store) subscriptionRequestTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, address string) (ids.UUID, bool, error) {
	entity, err := storekit.LegalEntityOf(ctx, tx, ids.UUID{})
	if err != nil {
		return ids.UUID{}, false, err
	}
	subject := subjectRef{Kind: entityContact, ID: contactID.String(), Address: address, SendingEntity: entity}
	request, found, err := newestRequest(ctx, tx, subject, subscriptionRequests, ids.UUID{})
	if err != nil || found {
		return request.event, found, err
	}
	event, found, err := inboundQualifyingEvent(ctx, tx, subject, time.Time{}, entity)
	if err != nil || !found {
		return ids.UUID{}, false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if err := RecordDerivedQualifyingEvent(ctx, tx, contactID.String(), event, by); err != nil {
		return ids.UUID{}, false, err
	}
	id, err := qualifyingEventRowTx(ctx, tx, contactID, event)
	return id, err == nil, err
}

// qualifyingEventRowTx names the row a derived event was stamped as.
func qualifyingEventRowTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, event QualifyingEvent) (ids.UUID, error) {
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
