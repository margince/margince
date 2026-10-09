// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An agent's correction of a company fact says an agent made it, and the
// automatic refresh leaves it alone the way it leaves a contact's.
//
// The two provenance columns answer different questions. captured_by names the
// identity that wrote the row. source names whose judgement the value carries,
// and source alone decides what the machine may replace.

import "testing"

func TestAnAgentCorrectionSaysAnAgentMadeIt(t *testing.T) {
	e := setupDedupe(t)
	companyID := evidenceCompany(e.as(), t, e)

	value := "Agent-corrected profile"
	out, err := e.store.UpdateCompanyProfileField(e.asAgent(), companyID, "icp",
		ProfileFieldWriteInput{Value: &value})
	if err != nil {
		t.Fatalf("agent correction: %v", err)
	}

	if string(out.Source) != "agent" {
		t.Errorf("source = %q, want agent: the granting human never saw this value", out.Source)
	}
	// Neither verified column is written: a row naming a verifier nobody
	// verified would record a confirmation that did not happen.
	if out.VerifiedBy != nil {
		t.Errorf("verified_by = %v on an agent write, want nothing", *out.VerifiedBy)
	}
	if out.VerifiedAt != nil {
		t.Errorf("verified_at = %v on an agent write, want nothing", *out.VerifiedAt)
	}
	if out.Value != value {
		t.Errorf("value = %q, want the agent's correction", out.Value)
	}
}

// A contact's own correction is unchanged by any of this.
func TestAHumanCorrectionStillNamesTheHumanWhoMadeIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := evidenceCompany(ctx, t, e)

	value := "Human-corrected profile"
	out, err := e.store.UpdateCompanyProfileField(ctx, companyID, "icp",
		ProfileFieldWriteInput{Value: &value})
	if err != nil {
		t.Fatalf("human correction: %v", err)
	}
	if string(out.Source) != "human" {
		t.Errorf("source = %q, want human", out.Source)
	}
	if out.VerifiedBy == nil || *out.VerifiedBy != e.rep.String() {
		t.Errorf("verified_by = %v, want the calling rep %v", out.VerifiedBy, e.rep)
	}
	if out.VerifiedAt == nil {
		t.Error("a human correction that records no time is not a confirmation")
	}
}
