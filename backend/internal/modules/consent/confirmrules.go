// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the recipient's country asks of a confirm link before it is minted. A
// details check may have to leave out the news question. A consent mail may
// have to answer the subject's own request.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subscriptionRequestKinds are the records that show the contact asking to
// hear from us. They are a logged request, a posted form and a written mail.
var subscriptionRequestKinds = []string{KindRequestedBySubject, KindInquiry, "inbound_message"}

// confirmRules is what the recipient's rules decide about one link.
type confirmRules struct {
	asksNews bool
	// subjectRequest is the request a consent link answers, zero when none is
	// on record.
	subjectRequest ids.UUID
}

// confirmRulesTx decides both for a contact's link. A consent link minted by
// an operator under rules that need a request is refused without one.
func confirmRulesTx(
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
	out.subjectRequest, err = subscriptionRequestTx(ctx, tx, contactID, address)
	if err != nil {
		return out, err
	}
	if found && rules.ConsentConfirmationNeedsRequest && out.subjectRequest.IsZero() {
		return out, &ValidationError{
			Field: contactIDKey,
			Reason: "this contact has not asked to subscribe, and their country's rules allow a " +
				"consent-confirmation mail only in answer to their own request. Record the request " +
				"(a form they posted, a mail they wrote, or a call you log) first",
		}
	}
	return out, nil
}

// subscriptionRequestTx is the newest live request from the contact on record
// for the address the link goes to, zero when there is none.
func subscriptionRequestTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, address string) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT q.id FROM consent_qualifying_event q
		 WHERE q.contact_id = $1 AND q.kind = ANY($2::text[]) AND q.retracted_at IS NULL
		   AND q.occurred_at <= $3 AND `+requestCoversSQL("q.recorded_for_addresses", "$4")+`
		 ORDER BY q.occurred_at DESC, q.id DESC LIMIT 1`,
		contactID, subscriptionRequestKinds, time.Now().Add(clockSkewAllowance), address).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, nil
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: reading whether the contact asked to subscribe: %w", err)
	}
	return id, nil
}
