// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/footerhealth"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func footerRead(t *testing.T) healthRead {
	t.Helper()
	reads := Server{}.healthReads()
	i := slices.IndexFunc(reads, func(r healthRead) bool { return r.id == footerhealth.SourceID })
	if i < 0 {
		t.Fatal("System health reads no footer source")
	}
	return reads[i]
}

// Only a seat that may fill in the company details is told they are missing.
// A seat that only reads the settings sees the source withheld and never
// reaches the read.
//
// Mutation: ask principal.ActionRead for the footer source and this fails.
func TestTheFooterWarningIsWithheldFromASeatThatCannotFillTheDetails(t *testing.T) {
	read := footerRead(t)
	ctx := healthTestContext(map[string]principal.ObjectGrant{"installation_settings": {Read: true}})
	report := composeHealth(ctx, []healthRead{read}, func() time.Time { return time.Time{} })
	if len(report.Sources) != 1 || report.Sources[0].Availability != healthWithheld || len(report.Findings) != 0 {
		t.Fatalf("a read-only seat got %+v, want the footer source withheld", report)
	}
	if report.Sources[0].Permission != "installation_settings:update" {
		t.Errorf("the withheld source names %q, want the grant it needs", report.Sources[0].Permission)
	}

	admin := healthTestContext(map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}})
	reached := false
	read.read = func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
		reached = true
		return healthSource(footerhealth.SourceID, footerhealth.Object), nil, nil
	}
	report = composeHealth(admin, []healthRead{read}, func() time.Time { return time.Time{} })
	if !reached || report.Sources[0].Permission != "installation_settings:update" {
		t.Errorf("an admin's footer source: reached=%v %+v", reached, report.Sources[0])
	}
}
