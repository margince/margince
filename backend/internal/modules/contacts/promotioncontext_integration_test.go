// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// releasedPromotion is the context an approval hands the promotion: the lead's
// version and the digest the approval pinned.
func (e *promoteConsentEnv) releasedPromotion(t *testing.T, lead ids.LeadID, in PromoteLeadInput) context.Context {
	t.Helper()
	var digest string
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		digest, err = e.store.PromotionContextDigestTx(e.ctx, tx, lead, in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := e.owner.QueryRow(context.Background(), `SELECT version FROM lead WHERE id = $1`, lead.UUID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return auth.WithReleasedRecordPins(e.ctx, auth.ReleasedRecordPins{Target: &auth.ReleasedRecordPin{
		EntityType: leadEntity, EntityID: lead.UUID, Version: version, ContextDigest: digest,
	}})
}

// An approval released while the lead's domain was blocked must not create the
// company the lifted block now allows. The write refuses it under the lead lock.
func TestAReleasedPromotionWhoseCompanyDecisionMovedWritesNothing(t *testing.T) {
	e := setupPromoteConsent(t)
	const domain = "pinned-works.test"
	lead := e.seedLead(t, "pat@"+domain)
	e.mustExec(t, `UPDATE lead SET company_name = 'Pinned Works' WHERE id = $1`, lead.UUID)
	in := PromoteLeadInput{Trigger: string(TriggerHumanQualify)}
	if _, err := e.store.SetDomainAdmission(e.ctx, domain, DomainSuppressed, "a supplier, not a customer"); err != nil {
		t.Fatal(err)
	}
	released := e.releasedPromotion(t, lead, in)
	if _, err := e.store.SetDomainAdmission(e.ctx, domain, DomainAdmitted, "they became a client"); err != nil {
		t.Fatal(err)
	}

	_, _, err := e.store.PromoteLead(released, lead, in)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("promoting on the old company decision = %v, want version skew", err)
	}
	var written int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM contact WHERE converted_from_lead_id = $1)
		     + (SELECT count(*) FROM company)
		     + (SELECT count(*) FROM lead WHERE id = $1 AND status = 'promoted')`, lead.UUID).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Fatalf("the refused promotion wrote %d rows", written)
	}

	if _, _, err := e.store.PromoteLead(e.releasedPromotion(t, lead, in), lead, in); err != nil {
		t.Fatalf("promoting on the current company decision: %v", err)
	}
}
