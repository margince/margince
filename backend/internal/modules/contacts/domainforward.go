// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const PendingForwardedIdentity = "forwarded_identity"

// DomainIdentityForwarded is shared by admission and review of earlier bindings.
func DomainIdentityForwarded(requestedDomain, effectiveURL string) bool {
	requested, valid := freemail.Hostname(requestedDomain)
	effective, err := url.Parse(effectiveURL)
	if !valid || effectiveURL == "" {
		return false
	}
	if err != nil {
		return true
	}
	answered, valid := freemail.Hostname(effective.Hostname())
	return !valid || requested != answered
}

type DomainForwardEvidence struct {
	ReadID          ids.UUID `json:"read_id"`
	RequestedDomain string   `json:"requested_domain"`
	EffectiveURL    string   `json:"effective_url"`
	RedirectChain   []string `json:"redirect_chain"`
}

func (s *Store) WithholdForwardedDomain(ctx context.Context, evidence DomainForwardEvidence) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return err
	}
	if !DomainIdentityForwarded(evidence.RequestedDomain, evidence.EffectiveURL) {
		return fmt.Errorf("contacts: forwarded identity requires different registrable domains: %w", apperrors.ErrInvalidArgument)
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	return s.withholdDomain(ctx, evidence.RequestedDomain, PendingForwardedIdentity, string(encoded), evidence.ReadID)
}

func (s *Store) withholdDomain(ctx context.Context, requestedDomain, pendingReason, evidence string, readID ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return withholdDomainTx(ctx, tx, requestedDomain, pendingReason, evidence, readID)
	})
}

func withholdDomainTx(ctx context.Context, tx pgx.Tx, requestedDomain, pendingReason, evidence string, readID ids.UUID) error {
	prior, known, err := readDispositionTx(ctx, tx, requestedDomain)
	if err != nil || !known || prior.Settled() || prior.Suppressed() {
		return err
	}
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	reason, proof, domain := arg(pendingReason), arg(evidence), arg(requestedDomain)
	var sourceRead *ids.UUID
	if !readID.IsZero() {
		sourceRead = &readID
	}
	source := arg(sourceRead)
	var id ids.UUID
	err = tx.QueryRow(ctx, `UPDATE company_domain_disposition
   SET pending_reason = `+reason+`, evidence = `+proof+`, site_read_id = COALESCE(`+source+`, site_read_id), next_attempt_at = NULL, updated_at = now()
   WHERE domain = `+domain+` AND status = 'pending' AND (pending_reason IS DISTINCT FROM `+reason+` OR evidence IS DISTINCT FROM `+proof+`) RETURNING id`, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = storekit.Audit(ctx, tx, "update", entityCompany, id,
		map[string]any{"pending_reason": prior.PendingReason, "evidence": prior.Evidence}, map[string]any{auditKeyDomain: requestedDomain, "pending_reason": pendingReason, "evidence": evidence})
	return err
}
