// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the recipient's country asks of a confirm link before it is minted. A
// details check may have to leave out the news question. A consent mail may
// have to answer the subject's own request.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// confirmRules is what the recipient's rules decide about one link.
type confirmRules struct {
	asksNews bool
	// subjectRequest is the request a consent link answers, zero when none is
	// on record.
	subjectRequest ids.UUID
}

// confirmRulesTx decides both for a contact's link. A consent link minted by
// an operator, for a purpose the rules ask a request for, is refused without
// one.
func (s *Store) confirmRulesTx(
	ctx context.Context, tx pgx.Tx, who subject, kind string, purpose askedPurpose, address string, operatorAsks bool,
) (confirmRules, error) {
	out := confirmRules{asksNews: true}
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: who.entityType, ID: who.id.String()})
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
	out.subjectRequest, asked, err = s.subscriptionRequestTx(ctx, tx, who, address)
	if err != nil {
		return out, err
	}
	gate := requestGate{
		needsRequest: found && rules.ConsentConfirmationNeedsRequest, classes: rules.ConsentRequestFirstFor, asked: asked,
	}
	if !mayAskByMail(purpose, gate) {
		return out, &NoSubjectRequestError{}
	}
	return out, nil
}

// askedPurpose is the purpose a consent link asks about, read once per mint.
// It is zero for a link that names none.
type askedPurpose struct {
	requiresDOI bool
	class       messagingrules.PurposeClass
}

// requestGate is what the recipient's rules say about a consent mail that a
// user asks for, and whether the contact's own request is on record.
type requestGate struct {
	// needsRequest is the pack's ConsentConfirmationNeedsRequest, and classes
	// its ConsentRequestFirstFor.
	needsRequest bool
	classes      []messagingrules.PurposeClass
	asked        bool
}

// refuses reports whether the rules refuse a consent mail about this class. A
// pack that needs the request and names no class needs it for every class.
func (g requestGate) refuses(class messagingrules.PurposeClass) bool {
	if !g.needsRequest || g.asked {
		return false
	}
	return len(g.classes) == 0 || slices.Contains(g.classes, class)
}

// mayAskByMail is whether a user may send this contact a consent mail about
// the purpose. The mint refuses what fails it. The no-basis warning offers
// only a purpose that passes, so it never offers a request the mint refuses.
func mayAskByMail(purpose askedPurpose, gate requestGate) bool {
	return confirmableByMail(purpose) && !gate.refuses(purpose.class)
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
// else a mail the contact wrote, read again from the timeline and stamped.
// found is false when there is none; request is zero for acquisition evidence,
// which has no qualifying-event row to name.
func (s *Store) subscriptionRequestTx(ctx context.Context, tx pgx.Tx, who subject, address string) (ids.UUID, bool, error) {
	ask, err := subjectAskTx(ctx, tx, who, address)
	if err != nil || !ask.found || !ask.fromMail {
		return ask.request, ask.found, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if err := RecordDerivedQualifyingEvent(ctx, tx, who.id.String(), ask.mail, by); err != nil {
		return ids.UUID{}, false, err
	}
	id, err := qualifyingEventRowTx(ctx, tx, ids.From[ids.ContactKind](who.id), ask.mail)
	return id, err == nil, err
}

// subjectAsk is the contact's own request as subjectAskTx finds it.
type subjectAsk struct {
	found bool
	// request is the recorded request's event, zero for acquisition evidence.
	request ids.UUID
	// fromMail marks a request that is only a mail the contact wrote, not yet
	// stamped as a qualifying event.
	fromMail bool
	mail     QualifyingEvent
}

// subjectAskTx finds the contact's own request without writing anything. The
// mail is the installation's own, so the default legal entity is the sender.
func subjectAskTx(ctx context.Context, tx pgx.Tx, who subject, address string) (subjectAsk, error) {
	entity, err := storekit.LegalEntityOf(ctx, tx, ids.UUID{})
	if err != nil {
		return subjectAsk{}, err
	}
	subject := subjectRef{Kind: who.entityType, ID: who.id.String(), Address: address, SendingEntity: entity}
	request, found, err := newestRequest(ctx, tx, subject, subscriptionRequests, ids.UUID{})
	if err != nil || found {
		return subjectAsk{found: found, request: request.event}, err
	}
	// A lead has no timeline of mails it wrote, so only a recorded request counts.
	if who.entityType == entityLead {
		return subjectAsk{}, nil
	}
	mail, found, err := inboundQualifyingEvent(ctx, tx, subject, time.Time{}, entity)
	return subjectAsk{found: found, fromMail: found, mail: mail}, err
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
