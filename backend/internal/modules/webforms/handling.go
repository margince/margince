// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// HandlingFence keeps a policy cutover outside an intake's committed writes.
type HandlingFence interface {
	Within(context.Context, ids.UUID, func(context.Context) error) error
}

// ErrHandlingBusy asks a sender to retry without acknowledging a submission.
var ErrHandlingBusy = errors.New("webforms: handling is busy")

// WithHandlingFence shares the form lock with the workflow consumer.
func (s *Store) WithHandlingFence(fence HandlingFence) *Store {
	clone := *s
	clone.fence = fence
	return &clone
}

func sameID(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func checkHandlingPolicy(ctx context.Context, tx pgx.Tx, formID ids.UUID, current, next Form) error {
	if sameID(current.HandlingAutomationID, next.HandlingAutomationID) {
		return nil
	}
	if err := auth.Require(ctx, "automation", principal.ActionUpdate); err != nil {
		return err
	}
	if next.HandlingAutomationID == nil {
		return nil
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM automation WHERE id = @automation AND key = @recipe
    AND params->>'form_id' = @form AND archived_at IS NULL
 )`, pgx.NamedArgs{"automation": *next.HandlingAutomationID, "recipe": provenance.WebsiteInquirySource, "form": formID.String()}).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return badInput(colHandlingAutomationID, "must name a live inquiry policy for this form")
	}
	return nil
}

// HandlingUnavailable is shared by form edits and the inquiry policy's HTTP write.
func HandlingUnavailable() error {
	return &httperr.DetailedError{Status: http.StatusServiceUnavailable, Code: httperr.WebsiteFormBusyCode, Detail: "this form is busy; try the same request again"}
}
