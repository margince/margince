// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The impersonation warning: the sign capture saw, how a contact read serves
// it, and how a user clears it.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The signs of impersonation capture screens a new sender for. The contract
// enum spells them, and contact.quarantine_reason stores the same codes.
const (
	impersonationPunycodeDomain         = string(crmcontracts.ContactImpersonationWarningReasonPunycodeDomain)
	impersonationDisplayNameOtherDomain = string(crmcontracts.ContactImpersonationWarningReasonDisplayNameOtherDomain)
)

// fieldImpersonationWarning is the wire field a cleared warning leaves.
const fieldImpersonationWarning = "impersonation_warning"

// auditImpersonationWarningCleared keys the audit image of a cleared warning.
// Its value is the reason cleared, or impersonationUnrecorded for a verdict with none.
const (
	auditImpersonationWarningCleared = "impersonation_warning_cleared"
	impersonationUnrecorded          = "unrecorded"
)

// impersonationWarningColumns projects the verdict a reader is still owed: the
// time is null once a user cleared it, and the reason rides beside it.
const impersonationWarningColumns = `CASE WHEN quarantine_cleared_at IS NULL THEN quarantined_at END AS impersonation_detected_at,
	quarantine_reason`

// impersonationSign names the sign a sender shows, or "" when it shows none.
// Capture still records a sender that shows one, because hiding a suspicious
// mail is worse than labelling it.
//
// Both signs are statements about the sender's mail domain. With no domain
// neither can apply, and an embedded address would otherwise differ from "".
func impersonationSign(displayName, domain string) string {
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "xn--") || strings.Contains(domain, ".xn--") {
		return impersonationPunycodeDomain
	}
	_, embedded, ok := strings.Cut(strings.ToLower(displayName), "@")
	if !ok {
		return ""
	}
	if end := strings.IndexAny(embedded, " >,;"); end >= 0 {
		embedded = embedded[:end]
	}
	embedded = strings.Trim(embedded, ".")
	if embedded == "" || embedded == strings.ToLower(domain) {
		return ""
	}
	return impersonationDisplayNameOtherDomain
}

// impersonationWarningOf is the wire warning for a scanned verdict, or nil when
// the contact carries none a reader is owed.
func impersonationWarningOf(detectedAt *time.Time, reason *string) *crmcontracts.ContactImpersonationWarning {
	if detectedAt == nil {
		return nil
	}
	warning := &crmcontracts.ContactImpersonationWarning{DetectedAt: *detectedAt}
	if reason != nil {
		r := crmcontracts.ContactImpersonationWarningReason(*reason)
		warning.Reason = &r
	}
	return warning
}

// ClearImpersonationWarning records a user's answer that the sender is fine.
//
// shownAt names the warning the user read, by its detection time. A contact
// that now carries another open warning is a conflict, so nobody clears a
// warning they were not shown. A contact with no open warning writes nothing.
// The verdict stays on the row beside the answer, as the record of it.
// Human-only: mail an agent reads could ask it to clear the warning about its sender.
func (s *Store) ClearImpersonationWarning(ctx context.Context, id ids.ContactID, shownAt time.Time) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return err
	}
	if shownAt.IsZero() {
		return httperr.Validation("detected_at", "required", "name the warning to clear by its detected_at")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, entityContact, id.UUID); err != nil {
			return err
		}
		// One statement decides: the row lock above keeps the open warning
		// from changing between the comparison and the write.
		var reason *string
		var stands bool
		err := tx.QueryRow(ctx, `
			WITH open AS (
				SELECT quarantined_at FROM contact
				 WHERE id = $1 AND quarantined_at IS NOT NULL AND quarantine_cleared_at IS NULL),
			cleared AS (
				UPDATE contact SET quarantine_cleared_at = now(), quarantine_cleared_by = $2
				  FROM open
				 WHERE contact.id = $1 AND open.quarantined_at = $3
				RETURNING coalesce(contact.quarantine_reason, $4) AS reason)
			SELECT (SELECT reason FROM cleared), EXISTS (SELECT 1 FROM open)`,
			id, by, shownAt, impersonationUnrecorded).Scan(&reason, &stands)
		if err != nil {
			return fmt.Errorf("contacts: clearing the impersonation warning: %w", err)
		}
		if reason == nil {
			if stands {
				return fmt.Errorf("the contact carries another open warning than the one shown: %w", apperrors.ErrConflict)
			}
			return nil
		}
		// An occurrence: the verdict and its reason stay on the row, so no
		// field holds a prior value for a before-image to name.
		auditID, err := storekit.AuditEvent(ctx, tx, "update", entityContact, id.UUID,
			map[string]any{auditImpersonationWarningCleared: *reason})
		if err != nil {
			return fmt.Errorf("contacts: recording the cleared impersonation warning: %w", err)
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
			ChangedFields: map[string]any{fieldImpersonationWarning: nil},
		})
	})
}

// auditImpersonationWarningCarried keys the audit image of a warning a merge
// moved to the survivor. Its value is the reason, as for a cleared one.
const auditImpersonationWarningCarried = "impersonation_warning_carried"

// carryImpersonationWarningTx moves an open warning from a merged-away contact
// to the survivor, whose addresses now include the suspect one.
//
// Without it a merge would answer the warning, and an agent may merge. A
// survivor with an open warning keeps its own. A cleared one yields, and the
// audit row of that clear still names who answered it.
func carryImpersonationWarningTx(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	var reason string
	err := tx.QueryRow(ctx, `
		UPDATE contact survivor
		   SET quarantined_at = merged.quarantined_at, quarantine_reason = merged.quarantine_reason,
		       quarantine_cleared_at = NULL, quarantine_cleared_by = NULL
		  FROM contact merged
		 WHERE survivor.id = $2 AND survivor.archived_at IS NULL AND merged.id = $1
		   AND merged.quarantined_at IS NOT NULL AND merged.quarantine_cleared_at IS NULL
		   AND (survivor.quarantined_at IS NULL OR survivor.quarantine_cleared_at IS NOT NULL)
		RETURNING coalesce(survivor.quarantine_reason, $3)`,
		sourceID, targetID, impersonationUnrecorded).Scan(&reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: carrying the impersonation warning to the survivor: %w", err)
	}
	// An occurrence on the survivor: the merge raised a warning its own
	// fields never held, so no before-image exists to name.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", entityContact, targetID.UUID,
		map[string]any{auditImpersonationWarningCarried: reason})
	if err != nil {
		return fmt.Errorf("contacts: recording the carried impersonation warning: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, targetID.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldImpersonationWarning: reason},
	})
}
