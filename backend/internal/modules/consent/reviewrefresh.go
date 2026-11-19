// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// MatchesAssessment compares the warnings the sender saw with the current send.
func (r Review) MatchesAssessment(set commsauthz.DecisionSet) bool {
	current := BlockingRefusals(set)
	if len(current) != len(r.Refusals) {
		return false
	}
	for i, now := range current {
		before := r.Refusals[i]
		if !sameRecipient(before, now) || before.ReasonCode != now.ReasonCode || before.PolicyRevision != now.PolicyRevision || before.Country != now.Country || before.CountrySource != now.CountrySource || !sameInstructionScope(before, now) {
			return false
		}
	}
	return true
}

// RefreshRefusal preserves the old decision and asks about the changed message.
func (g *Gate) RefreshRefusal(ctx context.Context, set commsauthz.DecisionSet, intent ids.UUID) (Review, error) {
	var out Review
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := CloseReviewForIntentTx(ctx, tx, intent, ReviewClosure{State: ReviewCancelled, Because: "the message assessment changed and needs a new confirmation"}); err != nil {
			return err
		}
		var err error
		out, err = OpenReviewTx(ctx, tx, set, intent)
		return err
	})
	return out, err
}

// AssessmentRevision binds the acknowledgement to the displayed warnings.
func (r Review) AssessmentRevision() string {
	var parts []string
	for _, refusal := range r.Refusals {
		parts = append(parts, refusal.Address, refusal.SubjectKind, refusal.SubjectID, refusal.ReasonCode, string(refusal.claim()), refusal.FooterDigest, refusal.PolicyRevision, refusal.Country, refusal.CountrySource)
		// The scope sameInstructionScope compares, by value: a pointer prints as an address.
		parts = append(parts, optionalIDText(refusal.SendingEntity), optionalIDText(refusal.PurposeID), optionalIDText(refusal.DealID))
		parts = append(parts, fmt.Sprintf("%q", refusal.FooterFindings), fmt.Sprintf("%q", refusal.FooterPreview))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q", parts))))
}

func optionalIDText(id *ids.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (r Review) checkAcknowledgement(revision string) error {
	for _, refusal := range r.Refusals {
		if (len(refusal.FooterFindings) > 0 || commsauthz.LevelForReason(refusal.ReasonCode) == commsauthz.LevelSubject) && revision != r.AssessmentRevision() {
			return &InstructionStaleError{Why: "the email warnings changed; read them again and confirm"}
		}
	}
	return nil
}
