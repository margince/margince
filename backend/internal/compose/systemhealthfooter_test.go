// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func footerRead(t *testing.T) healthRead {
	t.Helper()
	reads := Server{}.healthReads()
	i := slices.IndexFunc(reads, func(r healthRead) bool { return r.id == healthFooterID })
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
		return healthSource(healthFooterID, healthFooterObject), nil, nil
	}
	report = composeHealth(admin, []healthRead{read}, func() time.Time { return time.Time{} })
	if !reached || report.Sources[0].Permission != "installation_settings:update" {
		t.Errorf("an admin's footer source: reached=%v %+v", reached, report.Sources[0])
	}
}

func TestAFooterGapNamesTheEntityTheMissingDetailsAndTheHeldSends(t *testing.T) {
	branch := ids.NewV7()
	source := healthSource(healthFooterID, healthFooterObject)
	f := footerFinding(source, entityFooterGap{name: "Branch Co", FooterGap: consent.FooterGap{
		Entity: branch, Missing: []string{"controller_identity", "privacy_contact"}, HeldByPerson: 2, HeldInBackground: 1,
	}})
	if f.Code != footerIncompleteCode || !f.RequiresAction || f.Responsible != crmcontracts.HealthFindingResponsibleAdministrator {
		t.Errorf("finding %+v, want an administrator's action", f)
	}
	gap := f.FooterGap
	if gap == nil || gap.LegalEntityName != "Branch Co" || ids.UUID(gap.LegalEntityId) != branch ||
		!slices.Equal(gap.Missing, []crmcontracts.HealthFooterGapMissing{"controller_identity", "privacy_contact"}) ||
		gap.HeldByPerson != 2 || gap.HeldInBackground != 1 {
		t.Fatalf("footer gap %+v", gap)
	}
	if f.Count == nil || *f.Count != 3 || f.Impact != crmcontracts.HealthFindingImpactMeasured {
		t.Errorf("count %v impact %s, want 3 held messages measured", f.Count, f.Impact)
	}
	if f.Id == footerRead(t).id+":"+footerIncompleteCode {
		t.Error("the finding id does not name its entity, so two entities collide")
	}
}

// The button opens the page where the first missing detail is filled in.
func TestAFooterGapLeadsToWhereItsFirstDetailIsFilledIn(t *testing.T) {
	cases := []struct {
		name string
		gap  consent.FooterGap
		want crmcontracts.HealthActionDestination
	}{
		{"further entity without an address", consent.FooterGap{Missing: []string{"controller_identity"}}, crmcontracts.HealthActionDestinationLegalEntities},
		{"default entity without an address", consent.FooterGap{IsDefault: true, Missing: []string{"controller_identity"}}, crmcontracts.HealthActionDestinationDisclosures},
		{"default entity without a name", consent.FooterGap{IsDefault: true, Missing: []string{footerDetailEntityName}}, crmcontracts.HealthActionDestinationCompanyProfile},
		{"default entity without an address or a name", consent.FooterGap{IsDefault: true, Missing: []string{"controller_identity", footerDetailEntityName}}, crmcontracts.HealthActionDestinationDisclosures},
		{"privacy contact", consent.FooterGap{Missing: []string{"privacy_contact"}}, crmcontracts.HealthActionDestinationDisclosures},
	}
	for _, c := range cases {
		if got := footerDestination(entityFooterGap{FooterGap: c.gap}); got != c.want {
			t.Errorf("%s leads to %s, want %s", c.name, got, c.want)
		}
	}
}

// A seat that may fill the disclosure page but not the entity record sees the
// gap with its button off, instead of a button that leads nowhere it can save.
func TestAFooterButtonIsOffForASeatThatCannotChangeItsPage(t *testing.T) {
	ops := healthTestContext(map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}, "legal_entity": {Read: true}})
	if footerRemedyAllowed(ops, crmcontracts.HealthActionDestinationLegalEntities) {
		t.Error("a seat without legal_entity update may open the entity editor")
	}
	if !footerRemedyAllowed(ops, crmcontracts.HealthActionDestinationDisclosures) {
		t.Error("a seat that may change the settings cannot open the disclosure page")
	}
	editor := healthTestContext(map[string]principal.ObjectGrant{"legal_entity": {Update: true}})
	if !footerRemedyAllowed(editor, crmcontracts.HealthActionDestinationLegalEntities) {
		t.Error("a legal entity editor's button is off")
	}
}
