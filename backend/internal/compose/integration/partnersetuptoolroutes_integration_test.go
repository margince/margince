// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An assistant sets up a partner program through set_up_partner, a human
// approves it, and the margin tier stays unset. Driven through the real
// registry, store and approvals. The subject is the order of the checks:
// before an approval is staged, and again under the company lock.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const setUpPartnerVerb = "set_up_partner"

func (c *channelSendEnv) partnerSetUpInvoker(t *testing.T) func(string) (string, error) {
	t.Helper()
	return c.verbInvoker(t, c.mintPassport(t, []string{"read", "write"}), setUpPartnerVerb)
}

func (c *channelSendEnv) partnerRow(t *testing.T, company string) (kinds, certStatus string, tier *string, live bool) {
	t.Helper()
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT array_to_string(partner_kinds, ','), cert_status, margin_tier, archived_at IS NULL FROM partner WHERE company_id = $1`,
			company).Scan(&kinds, &certStatus, &tier, &live)
	}); err != nil {
		t.Fatalf("reading the partner row of %s: %v", company, err)
	}
	return kinds, certStatus, tier, live
}

func (c *channelSendEnv) hasPartnerRow(t *testing.T, company string) bool {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM partner WHERE company_id = $1`, company).Scan(&n)
	}); err != nil {
		t.Fatalf("counting partner rows: %v", err)
	}
	return n > 0
}

func (c *channelSendEnv) humanSetsUpPartner(t *testing.T, company, tier string) {
	t.Helper()
	body := AnyMap{"partner_role": "hosting"}
	if tier != "" {
		body["margin_tier"] = tier
	}
	if status := c.Call(t, "PUT", "/v1/companies/"+company+"/partner", body, nil, nil); status != http.StatusOK {
		t.Fatalf("human partner set-up → %d", status)
	}
}

func setUpArgs(company string) string {
	return fmt.Sprintf(`{"company_id":%q,"partner_role":"consulting","relationship_stage":"contacted","next_step":"Send the terms"}`, company)
}

func setUpRetry(company, approval string) string {
	return fmt.Sprintf(`{"company_id":%q,"partner_role":"consulting","relationship_stage":"contacted","next_step":"Send the terms","approval_id":%q}`, company, approval)
}

func TestAnApprovedAssistantSetUpStartsAProgramWithNoTier(t *testing.T) {
	c := setupChannelSendNoGmail(t)
	invoke := c.partnerSetUpInvoker(t)
	company := c.enrichTarget(t)

	staged := c.stagedApproval(t, invoke, setUpArgs(company))
	if c.hasPartnerRow(t, company) {
		t.Fatal("staging the set-up already wrote a partner row")
	}
	if status := c.Call(t, "POST", "/v1/approvals/"+staged.ApprovalID.String()+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	if _, err := invoke(setUpRetry(company, staged.ApprovalID.String())); err != nil {
		t.Fatalf("approved retry → %v", err)
	}

	role, cert, tier, live := c.partnerRow(t, company)
	if role != "consulting" || cert != "applied" || tier != nil || !live {
		t.Errorf("program = role %q cert %q tier %v live %v, want consulting, applied, no tier, live", role, cert, tier, live)
	}
	if status := c.Call(t, "PATCH", "/v1/companies/"+company,
		AnyMap{"relationship_types": []string{"partner"}}, nil, nil); status != http.StatusOK {
		t.Errorf("naming the company a partner after the set-up → %d, want 200", status)
	}
}

func TestAnAssistantSetUpNamingATierStagesNothing(t *testing.T) {
	c := setupChannelSendNoGmail(t)
	invoke := c.partnerSetUpInvoker(t)
	company := c.enrichTarget(t)

	_, err := invoke(fmt.Sprintf(`{"company_id":%q,"partner_role":"hosting","margin_tier":"tier3_25"}`, company))
	if err == nil || !strings.Contains(err.Error(), "a human sets it on the company's Partner tab") {
		t.Fatalf("a call naming margin_tier → %v, want the human-only refusal", err)
	}
	c.assertApprovalCountOf(t, setUpPartnerVerb, 0, "a refused tier")
}

func TestAnAssistantSetUpOnAnExistingProgramIsRefusedBeforeStaging(t *testing.T) {
	c := setupChannelSendNoGmail(t)
	invoke := c.partnerSetUpInvoker(t)
	company := c.enrichTarget(t)
	c.humanSetsUpPartner(t, company, "tier1_15")

	_, err := invoke(setUpArgs(company))
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("set-up on a live program → %v, want a conflict", err)
	}
	c.assertApprovalCountOf(t, setUpPartnerVerb, 0, "a refused set-up")
}

func TestASetUpApprovedBeforeAHumanSetsUpTheProgramIsRefusedAndChangesNothing(t *testing.T) {
	c := setupChannelSendNoGmail(t)
	invoke := c.partnerSetUpInvoker(t)
	company := c.enrichTarget(t)

	staged := c.stagedApproval(t, invoke, setUpArgs(company))
	if status := c.Call(t, "POST", "/v1/approvals/"+staged.ApprovalID.String()+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	c.humanSetsUpPartner(t, company, "tier2_20")

	_, err := invoke(setUpRetry(company, staged.ApprovalID.String()))
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("redeeming after a human set the program up → %v, want a conflict", err)
	}
	role, _, tier, _ := c.partnerRow(t, company)
	if role != "hosting" || tier == nil || *tier != "tier2_20" {
		t.Errorf("the human's program became role %q tier %v, want hosting and tier2_20", role, tier)
	}
}

func TestASetUpOnAnArchivedProgramStartsFromTheDefaults(t *testing.T) {
	c := setupChannelSendNoGmail(t)
	invoke := c.partnerSetUpInvoker(t)
	company := c.enrichTarget(t)
	c.humanSetsUpPartner(t, company, "tier3_25")
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`UPDATE partner SET archived_at = now(), cert_status = 'certified', certified_staff = 7, retention_rate = 90,
			        served_segments = ARRAY['smb'], partner_fit_score = 80, partner_fit_score_computed = 40,
			        partner_fit_override_reason = 'human call', relationship_stage = 'active'
			  WHERE company_id = $1`, company); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE company_relationship_type SET archived_at = now() WHERE company_id = $1 AND relationship_type = 'partner'`, company)
		return err
	}); err != nil {
		t.Fatalf("archiving the program: %v", err)
	}

	staged := c.stagedApproval(t, invoke, setUpArgs(company))
	if status := c.Call(t, "POST", "/v1/approvals/"+staged.ApprovalID.String()+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	if _, err := invoke(setUpRetry(company, staged.ApprovalID.String())); err != nil {
		t.Fatalf("approved retry → %v", err)
	}
	if _, _, tier, live := c.partnerRow(t, company); tier != nil || !live {
		t.Errorf("revived program has tier %v live %v, want no tier and live", tier, live)
	}
	var cert, stage string
	var staff int16
	var retention, fit *int16
	var segments []string
	var reason *string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT cert_status, certified_staff, retention_rate, served_segments, partner_fit_score,
			        partner_fit_override_reason, relationship_stage FROM partner WHERE company_id = $1`,
			company).Scan(&cert, &staff, &retention, &segments, &fit, &reason, &stage)
	}); err != nil {
		t.Fatalf("reading the revived program: %v", err)
	}
	if cert != "applied" || staff != 0 || retention != nil || len(segments) != 0 || fit != nil || reason != nil || stage != "contacted" {
		t.Errorf("revived program kept old terms: cert %q staff %d retention %v segments %v fit %v reason %v stage %q",
			cert, staff, retention, segments, fit, reason, stage)
	}
}
