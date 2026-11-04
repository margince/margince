// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// admitAsk records an agent's note as model prose, so it never reads as typed by
// a user. It defaults an unstated enum and refuses a stated one the contract does
// not admit. Unstated is a contact typing with no fallback.
func admitAsk(ctx context.Context, req NewRequest) (NewRequest, error) {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalAgent && (req.NoteGeneratedBy == "" || req.NoteGeneratedBy == "human") {
		req.NoteGeneratedBy, req.NoteAIGenerated = "model", true
	}
	if req.NoteGeneratedBy == "" {
		req.NoteGeneratedBy = "human"
	}
	if req.NoteGeneratedBy == "model" {
		req.NoteAIGenerated = true
	}
	if req.FallbackPolicy == "" {
		req.FallbackPolicy = "none"
	}
	return req, validateAsk(req)
}

// askFieldRequired is the refusal code of an empty required field.
const askFieldRequired = "required"

// validateAsk refuses what the contract does not admit. The enums are
// refused rather than defaulted: an unknown note origin recorded as `human`
// would disclose model prose as typed.
func validateAsk(req NewRequest) error {
	if req.InternalReason == "" {
		return &values.ParseError{Field: "internal_reason", Code: askFieldRequired, Message: "internal_reason is required"}
	}
	if err := tooLong("internal_reason", req.InternalReason, reasonBound); err != nil {
		return err
	}
	if err := tooLong("value_for_target", req.ValueForTarget, reasonBound); err != nil {
		return err
	}
	if err := tooLong("forwardable_note", req.ForwardableNote, noteBound); err != nil {
		return err
	}
	if !crmcontracts.IntroNoteOrigin(req.NoteGeneratedBy).Valid() {
		return notAllowed("note_generated_by", "human, model or deterministic")
	}
	if !crmcontracts.IntroFallbackPolicy(req.FallbackPolicy).Valid() {
		return notAllowed("fallback_policy", "none, name_drop or next_route")
	}
	return nil
}

func notAllowed(field, allowed string) error {
	return &values.ParseError{
		Field: field, Code: "value_not_allowed",
		Message: fmt.Sprintf("%s must be %s", field, allowed),
	}
}
