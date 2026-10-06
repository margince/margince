// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// A promise card that asks instead of nagging. When we wrote to the contact
// after a promise was made, the email may have kept it; the card says so and
// offers Done and Not yet. It never closes the promise itself: whether an
// email delivered what was promised is the reader's call.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// wroteTo is the newest email we sent the contact.
type wroteTo struct {
	id      crmcontracts.Id
	at      time.Time
	subject string
}

// lastWroteTo reads the newest email the workspace itself sent to the
// contact; false when there is none.
//
// Only mail the provider filed as sent by us counts: a message whose From
// merely names our mailbox proves nothing, which is the rule the answered
// predicate holds too. A campaign and a notice the installation owes somebody
// are not a rep keeping a promise, so neither counts. The card names the email
// and its day, so it reads under the reader's content scope: an email they
// cannot open is never cited to them.
func lastWroteTo(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time, opts AssembleOptions) (wroteTo, bool, error) {
	if err := requireRead(ctx, "activity"); err != nil {
		return wroteTo{}, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	reaches := fmt.Sprintf(contactReachesActivity, bind(arg(contactID)))
	scope, err := activityScope(ctx, arg)
	if err != nil {
		return wroteTo{}, false, err
	}
	var sent wroteTo
	err = tx.QueryRow(ctx, `SELECT a.id, a.occurred_at, coalesce(a.subject, '')
		FROM activity a
		WHERE a.archived_at IS NULL AND `+reaches+` AND (`+scope+`)`+projectScope(opts, arg)+`
		AND a.kind = 'email' AND a.direction = 'outbound'
		AND a.counterparty_outbound_attested AND NOT a.bulk_mail_attested`+
		auth.OriginIsEngagement("a")+`
		AND a.occurred_at <= `+bind(arg(now))+`
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT 1`, args...).Scan(&sent.id, &sent.at, &sent.subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return wroteTo{}, false, nil
	}
	if err != nil {
		return wroteTo{}, false, fmt.Errorf("read the newest email to the contact: %w", err)
	}
	return sent, true, nil
}

// recordZone is the installation's zone, which the day a card names is read
// in, so every colleague sees the same day.
func recordZone(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	name, err := identity.TimezoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	return storekit.LoadZone(name)
}

// owedPromise is the promise a card speaks for and when it was made.
type owedPromise struct {
	kind   crmcontracts.ContactMomentMayBeDonePromiseType
	id     crmcontracts.Id
	madeAt time.Time
}

// promiseBehind finds the task or claim a promise card was built from. A card
// from any other rung has none.
func promiseBehind(moment crmcontracts.ContactMoment, page *crmcontracts.Contact360) (owedPromise, bool) {
	if moment.ClaimKey == overdueTaskKey || moment.ClaimKey == openTaskKey {
		if page.NextSteps == nil || len(moment.Evidence) == 0 || moment.Evidence[0].Id == nil {
			return owedPromise{}, false
		}
		for _, task := range page.NextSteps.Data {
			if task.Id == *moment.Evidence[0].Id {
				return owedPromise{kind: crmcontracts.ContactMomentMayBeDonePromiseTypeTask, id: task.Id, madeAt: task.OccurredAt}, true
			}
		}
		return owedPromise{}, false
	}
	if page.Claims == nil {
		return owedPromise{}, false
	}
	for _, claim := range *page.Claims {
		if moment.ClaimKey != claimMomentKey(overdueClaimRung, claim) && moment.ClaimKey != claimMomentKey(openClaimRung, claim) {
			continue
		}
		// Made when it was said, not when the extractor filed it; a claim with
		// no moment cannot be placed against an email, so it is not asked about.
		if claim.OccurredAt == nil {
			return owedPromise{}, false
		}
		return owedPromise{kind: crmcontracts.ContactMomentMayBeDonePromiseTypeClaim, id: claim.Id, madeAt: *claim.OccurredAt}, true
	}
	return owedPromise{}, false
}

// mayBeDone turns a promise card into the question whether our last email
// kept it, when that email went out strictly after the promise was made.
func mayBeDone(moment crmcontracts.ContactMoment, page *crmcontracts.Contact360, sent *wroteTo, zone *time.Location) (crmcontracts.ContactMoment, bool) {
	if sent == nil {
		return crmcontracts.ContactMoment{}, false
	}
	promise, ok := promiseBehind(moment, page)
	if !ok || !sent.at.After(promise.madeAt) {
		return crmcontracts.ContactMoment{}, false
	}
	label := sent.subject
	if label == "" {
		label = "Your email"
	}
	at := sent.at
	email := sent.id
	// The email is in the fingerprint, so Not yet holds only until we write
	// to them again.
	evidence := []crmcontracts.ContactMomentEvidence{moment.Evidence[0], {
		Type:       crmcontracts.ContactMomentEvidenceTypeActivity,
		Id:         &email,
		Label:      label,
		ObservedAt: &at,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:may_be_done:" + promise.id.String(),
		Rule:                moment.Rule,
		RuleVersion:         moment.RuleVersion,
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf("You may have done this — you wrote to them on %s", at.In(zone).Format("2 Jan")),
		WhyNow:              moment.Headline + ". " + moment.WhyNow,
		// An inference from timing, not an observed fact: the email may be
		// about something else entirely.
		Confidence:  crmcontracts.ContactMomentConfidenceMedium,
		Evidence:    evidence,
		FreshnessAt: &at,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindCompleteTask,
			Label: "Done",
			State: crmcontracts.ContactMomentActionStateAvailable,
		},
		MayBeDone: &crmcontracts.ContactMomentMayBeDone{
			PromiseType:     promise.kind,
			PromiseId:       promise.id,
			EmailActivityId: email,
			WroteAt:         at,
		},
	}, true
}

// proposer asks the may-be-done question in place of a promise card, unless
// the reader already answered Not yet to it. Not yet dismisses the question,
// so the plain promise card comes back rather than the walk moving past the
// promise.
func proposer(
	page *crmcontracts.Contact360, sent *wroteTo, zone *time.Location, dismissed func(crmcontracts.ContactMoment) bool,
) func(crmcontracts.ContactMoment) crmcontracts.ContactMoment {
	return func(moment crmcontracts.ContactMoment) crmcontracts.ContactMoment {
		if question, ok := mayBeDone(moment, page, sent, zone); ok && !dismissed(question) {
			return question
		}
		return moment
	}
}

// proposing is a rung whose promise card becomes the question whenever
// propose says so.
func proposing(rung ladderRung, propose func(crmcontracts.ContactMoment) crmcontracts.ContactMoment) ladderRung {
	return func(ctx context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
		moment, ok := rung(ctx, now, page)
		if !ok {
			return moment, false
		}
		return propose(moment), true
	}
}
