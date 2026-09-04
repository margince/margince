// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Asking whether a message may be sent, before there is a message.
//
// The per-person guard answers "may we write to this person, per purpose". That
// is the right question for a record page and the wrong one for a composer: the
// engine resolves a category from the THREAD a message answers, the deal or
// invoice it names and the evidence the sender offers, none of which belong to
// a person in the abstract. So a composer that only had the guard would show a
// verdict about a different question than the one the send asks.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// errNoPreviewAuthority is the composition defect of a preview surface built
// with no engine behind it. No sentinel, for the same reason
// errNoDeliveryStager carries none: it is a wiring fault and must surface as
// the 500 it is rather than borrow a refusal that would tell the caller
// something untrue about their request.
var errNoPreviewAuthority = errors.New("activities: preview has no consent authority wired")

// SendPreviewer answers what the engine would decide, and records nothing.
//
// Injected because consent owns the engine and a module never imports a
// sibling. The consent gate's own Preview satisfies it.
type SendPreviewer interface {
	Preview(ctx context.Context, req commsauthz.Request) (commsauthz.DecisionSet, error)
}

// PreviewSendInput is a message a composer has not written yet.
//
// No subject and no body. Neither reaches a consent decision — the engine reads
// recipients, the anchor, the links and the evidence — and asking a rep to write
// the mail before learning whether they may send it is the wrong way round.
type PreviewSendInput struct {
	Recipients []string
	Context    commsauthz.Category
	// LegacyPurposeKey is carried for one reason: the SEND still consults it
	// where the record supports no category on its own, so a preview that
	// could not pass it would answer a different question than the send it
	// previews — and then disagree with it, which is the failure this endpoint
	// exists to prevent.
	LegacyPurposeKey string
	MarketingPurpose string
	Evidence         commsauthz.Evidence
}

// PreviewSend answers what the engine would decide for this message.
//
// The origin resolves FIRST, exactly as a real send resolves it: a caller who
// names an anchor or a record they cannot read gets the row-scope answer here,
// before anything is asked about anybody's consent. Without that a caller could
// name a stranger's deal and have the engine answer about it — an unauthorized
// read wearing a preview.
//
// It runs on the store's own transaction through the previewer, which rolls
// back. Nothing is recorded: no decision rows, and no lawful basis, because a
// basis is the ground a SEND relies on and nothing has been sent.
func (s *Store) PreviewSend(ctx context.Context, origin SendOrigin, in PreviewSendInput, previewer SendPreviewer) (commsauthz.DecisionSet, error) {
	// THE SAME GRANT THE SEND NEEDS, and asked first.
	//
	// A preview answers about a message this caller could send, so a caller who
	// could not send must not be able to ask. Without it the endpoint is a
	// consent oracle for anyone with a session: name an address, learn whether
	// that person has objected. The record probe below is not a substitute — it
	// gates the RECORDS named, and a reply preview names none.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	if previewer == nil {
		// A preview with no authority behind it would answer "allowed" about a
		// question nobody asked. Refused the way the send path refuses a
		// missing gate: absence of the engine never reads as permission.
		return commsauthz.DecisionSet{}, errNoPreviewAuthority
	}
	links, err := origin.resolve(ctx, s)
	if err != nil {
		return commsauthz.DecisionSet{}, err
	}
	return previewer.Preview(ctx, commsauthz.Request{
		Recipients:       connector.EmailRecipients(in.Recipients),
		Context:          in.Context,
		LegacyPurposeKey: in.LegacyPurposeKey,
		MarketingPurpose: in.MarketingPurpose,
		Evidence:         in.evidenceWithAnchor(origin),
		AnchorActivityID: origin.anchor.UUID,
		Links:            linkedRecordIDs(links),
	})
}

// evidenceWithAnchor fills the anchor in the way outboundMessage.evidence does
// for a real send, so a preview and the send it previews offer the engine the
// same evidence rather than differing by one field nobody set.
func (in PreviewSendInput) evidenceWithAnchor(origin SendOrigin) commsauthz.Evidence {
	e := in.Evidence
	if e.ActivityID == (ids.UUID{}) {
		e.ActivityID = origin.anchor.UUID
	}
	return e
}
