// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fieldAIAssistedOn = "ai_assisted_on"

// AIAssistedError refuses an ai_assisted mark Polish wording cannot have set.
type AIAssistedError struct {
	Field  string
	Reason string
}

var _ apperrors.FieldFault = (*AIAssistedError)(nil)

func (e *AIAssistedError) Error() string { return e.Field + ": " + e.Reason }

// FieldFault names the field the mark was refused on.
func (e *AIAssistedError) FieldFault() (field, code, message string) {
	return e.Field, faultInvalid, e.Reason
}

// refuseMarkByMachine admits the mark from a human alone. Polish wording is a
// button a human presses; an agent or a connector writes its own words.
func refuseMarkByMachine(ctx context.Context, marked bool) error {
	if !marked {
		return nil
	}
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalHuman {
		return nil
	}
	return &AIAssistedError{Field: fieldAIAssisted, Reason: "only a human sets ai_assisted, through Polish wording"}
}

// checkLoggedMark admits a marked activity only on the record its words were
// polished on: one of its links, or the record its form started on. Polish
// wording asked that record's restriction.
func checkLoggedMark(ctx context.Context, in LogActivityInput) error {
	if err := refuseMarkByMachine(ctx, in.AIAssisted); err != nil || !in.AIAssisted {
		return err
	}
	if in.AIAssistedOn == nil || !slices.Contains(in.Links, *in.AIAssistedOn) && !startedOn(in, *in.AIAssistedOn) {
		return &AIAssistedError{Field: fieldAIAssistedOn, Reason: "ai_assisted_on must name one of links or the record the form started on, where the words were polished"}
	}
	return nil
}

// startedOn reports whether the record is the one a call or meeting form
// started on: a company page files its attendees, not the company.
func startedOn(in LogActivityInput, record ActivityLinkInput) bool {
	return in.DealFiling != nil && string(in.DealFiling.RecordType) == record.EntityType &&
		ids.UUID(in.DealFiling.RecordId) == record.EntityID
}
