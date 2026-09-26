// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince's offer in a company conversation: "The imprint names Acme Robotics
// GmbH — shall I use that as the legal name?" A bare yes to it grants exactly
// that field and value, and nothing else.
//
// The grant widens what a model reply can authorize, so every part of it is the
// server's rather than the conversation's. The offer is a structured entry in
// the reply, grounded in the dossier like any proposed change, and recorded by
// the server when Margince makes it; it is never read out of prose, and the
// client's replay of the conversation carries none. It stands only while the
// replayed conversation ends on the very assistant turn that made it, over the
// same dossier draft, for the human it was made to — and every answered message
// replaces it, so it is good for one reply.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// companyReadOfferLimit is how many values one reply may offer. One, because a
// bare yes has to name a single thing to be agreement with it.
const companyReadOfferLimit = 1

// companyReadOffer is the value a reply offers to apply, in the reply's own
// shape, and the standing offer the next request shows the model.
type companyReadOffer struct {
	Field     string   `json:"field"`
	Value     string   `json:"value"`
	SourceIDs []string `json:"source_ids"`
}

// validateCompanyReadOffers holds an offer to what a dossier-derived proposed
// change must satisfy — a known field, a value, and citations that state it —
// with no room for an administrator statement or a synthesis: an offer is
// Margince's, so the dossier is its only ground.
func validateCompanyReadOffers(offers []companyReadOffer, globalSources map[string]struct{}, known map[string]companyReadEvidence) error {
	if len(offers) > companyReadOfferLimit {
		return fmt.Errorf("compose: company read answer offers more than %d value", companyReadOfferLimit)
	}
	for _, offer := range offers {
		if !crmcontracts.CompanySiteReadSuggestedChangeField(offer.Field).Valid() {
			return fmt.Errorf("compose: company read answer offers unsupported field %q", clampToken(offer.Field))
		}
		if strings.TrimSpace(offer.Value) == "" {
			return fmt.Errorf("compose: company read answer offers an empty value")
		}
		sources, err := validateCompanyReadSourceIDs(offer.SourceIDs, known)
		if err != nil {
			return err
		}
		supported, err := citedValueSupported("offer", offer.Value, sources, globalSources, known)
		if err != nil {
			return err
		}
		if !supported {
			return fmt.Errorf("compose: company read offer value is not supported by its cited evidence")
		}
	}
	return nil
}

// withStandingOffer grants the standing offer's exact pair when the current
// message is a bare agreement. Anything more than agreement — "yes, but use
// Acme AG" — grants nothing here and answers to the correction path instead.
func (a companyChangeAuthorization) withStandingOffer(offer *companyReadOffer) companyChangeAuthorization {
	if offer != nil && isCompanyChangeConfirmation(a.currentMessage) {
		a.acceptedOffer = exactGrant{field: offer.Field, value: strings.TrimSpace(offer.Value)}
	}
	return a
}

// offerAccepted reports whether a reply turned the standing offer into the
// change it granted, which is what the offer slot's audit names.
func offerAccepted(offer *companyReadOffer, message string, reply companyReadModelReply) bool {
	grant := companyChangeAuthorization{currentMessage: message}.withStandingOffer(offer).acceptedOffer
	for _, change := range reply.ProposedChanges {
		if grant.covers(change) {
			return true
		}
	}
	return false
}

// exactGrant is authority over one field with one value, compared after
// trimming and nothing else — what a clicked clarify option and an accepted
// offer each confer.
type exactGrant struct{ field, value string }

func (g exactGrant) covers(change companyReadProposedChange) bool {
	return g.field != "" && change.Field == g.field && strings.TrimSpace(change.Value) == g.value
}

// offerTurnDigest names an assistant message by its content, trimmed the way
// both the reply and the replayed turn are.
func offerTurnDigest(message string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(message)))
	return hex.EncodeToString(sum[:])
}

// recordedOffer is the offer as the slot keeps it: bound to the assistant
// message that made it and the dossier draft that grounded it.
func recordedOffer(offer companyReadOffer, turnMessage string, draftVersion int) contacts.SiteReadOffer {
	return contacts.SiteReadOffer{
		Field: offer.Field, Value: strings.TrimSpace(offer.Value), SourceIDs: offer.SourceIDs,
		TurnDigest: offerTurnDigest(turnMessage), DraftVersion: draftVersion,
	}
}

// standingOffer answers the recorded offer only while the conversation ends on
// the assistant turn that made it, over the draft it was grounded in. A reply
// in between, a reworded turn, or a re-read dossier each leave nothing standing.
func standingOffer(recorded *contacts.SiteReadOffer, history []model.Message, draftVersion int) *companyReadOffer {
	if recorded == nil || len(history) == 0 || recorded.DraftVersion != draftVersion {
		return nil
	}
	last := history[len(history)-1]
	if last.Role != string(crmcontracts.CompanySiteReadConversationTurnRoleAssistant) ||
		offerTurnDigest(last.Content) != recorded.TurnDigest {
		return nil
	}
	return &companyReadOffer{Field: recorded.Field, Value: recorded.Value, SourceIDs: recorded.SourceIDs}
}

// siteReadOfferStore is the offer slot a conversation reads before it asks the
// model and replaces after the answer is accepted.
type siteReadOfferStore interface {
	StandingSiteReadOffer(ctx context.Context, readID ids.UUID) (*contacts.SiteReadOffer, error)
	ReplaceSiteReadOffer(ctx context.Context, readID ids.UUID, offer, accepted *contacts.SiteReadOffer) error
}

// offerTurn is one message's hold on the slot. The zero value holds nothing
// and records nothing: no store is wired, the conversation has no dossier, or
// the caller could not save what an offer proposes.
type offerTurn struct {
	store        siteReadOfferStore
	readID       ids.UUID
	draftVersion int
	recorded     *contacts.SiteReadOffer
	standing     *companyReadOffer
}

func beginOfferTurn(ctx context.Context, store siteReadOfferStore, read *contacts.SiteRead, history []model.Message) (offerTurn, error) {
	if store == nil || read == nil {
		return offerTurn{}, nil
	}
	recorded, err := store.StandingSiteReadOffer(ctx, read.ID)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// A reader who cannot save a company change is offered none; the
		// conversation still answers them.
		return offerTurn{}, nil
	}
	if err != nil {
		return offerTurn{}, err
	}
	turn := offerTurn{store: store, readID: read.ID, draftVersion: read.DraftVersion, recorded: recorded}
	turn.standing = standingOffer(recorded, history, read.DraftVersion)
	return turn, nil
}

// finish replaces the slot with the offer this reply made, if the caller may
// take it, and names the standing offer the reply accepted.
func (t offerTurn) finish(ctx context.Context, message string, reply companyReadModelReply) error {
	if t.store == nil {
		return nil
	}
	var next *contacts.SiteReadOffer
	if len(reply.Offers) > 0 && contacts.MayOfferSiteReadChange(ctx, reply.Offers[0].Field) {
		offer := recordedOffer(reply.Offers[0], reply.Message, t.draftVersion)
		next = &offer
	}
	if next == nil && t.recorded == nil {
		return nil
	}
	var accepted *contacts.SiteReadOffer
	if offerAccepted(t.standing, message, reply) {
		accepted = t.recorded
	}
	return t.store.ReplaceSiteReadOffer(ctx, t.readID, next, accepted)
}

// offerStore answers the engine's offer slot, nil when no dossier store is
// wired — a nil *contacts.Store must not become a non-nil interface.
//
//nolint:ireturn // the slot is consumed through its interface so the onboarding assistant can share it
func (e *deepReadEngine) offerStore() siteReadOfferStore {
	if e.contacts == nil {
		return nil
	}
	return e.contacts
}
