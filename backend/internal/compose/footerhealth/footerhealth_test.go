// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package footerhealth

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

func seat(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test", Permissions: principal.Permissions{Objects: objects}})
}

func TestAFooterGapNamesTheEntityTheMissingDetailsAndTheHeldSends(t *testing.T) {
	branch := ids.NewV7()
	base := crmcontracts.HealthFinding{Id: "footer:footer_incomplete", Code: IncompleteCode, RequiresAction: true}
	f := Finding(seat(nil), base, Gap{
		Name: "Branch Co", Entity: branch, Missing: []string{"privacy_contact"}, HeldByUser: 2, HeldInBackground: 1,
	})
	gap := f.FooterGap
	if gap == nil || gap.LegalEntityName != "Branch Co" || ids.UUID(gap.LegalEntityId) != branch ||
		!slices.Equal(gap.Missing, []crmcontracts.HealthFooterGapMissing{"privacy_contact"}) ||
		gap.HeldByUser != 2 || gap.HeldInBackground != 1 {
		t.Fatalf("footer gap %+v", gap)
	}
	if f.Responsible != crmcontracts.HealthFindingResponsibleAdministrator || f.Impact != crmcontracts.HealthFindingImpactMeasured {
		t.Errorf("finding %+v, want an administrator's measured gap", f)
	}
	if f.Id == base.Id {
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
		{"default entity without a name", consent.FooterGap{IsDefault: true, Missing: []string{EntityName}}, crmcontracts.HealthActionDestinationCompanyProfile},
		{"default entity without an address or a name", consent.FooterGap{IsDefault: true, Missing: []string{"controller_identity", EntityName}}, crmcontracts.HealthActionDestinationDisclosures},
		{"privacy contact", consent.FooterGap{Missing: []string{"privacy_contact"}}, crmcontracts.HealthActionDestinationDisclosures},
	}
	for _, c := range cases {
		if got := Destination(Gap{FooterGap: c.gap}); got != c.want {
			t.Errorf("%s leads to %s, want %s", c.name, got, c.want)
		}
	}
}

// A seat that may fill the disclosure page but not the entity record sees the
// gap with its button off. The button would lead to a page it cannot save.
func TestAFooterButtonIsOffForASeatThatCannotChangeItsPage(t *testing.T) {
	ops := seat(map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}, "legal_entity": {Read: true}})
	onEntity := Gap{Missing: []string{"controller_identity"}}
	if f := Finding(ops, crmcontracts.HealthFinding{}, onEntity); f.Action.Available {
		t.Error("a seat without legal_entity update may open the entity editor")
	}
	if !remedyAllowed(ops, crmcontracts.HealthActionDestinationDisclosures) {
		t.Error("a seat that may change the settings cannot open the disclosure page")
	}
	editor := seat(map[string]principal.ObjectGrant{"legal_entity": {Update: true}})
	if !remedyAllowed(editor, crmcontracts.HealthActionDestinationLegalEntities) {
		t.Error("a legal entity editor's button is off")
	}
}

// Only a seat that may fill in the company details is told they are missing.
// A seat that only reads the settings sees the source withheld, and no
// database is asked.
//
// Mutation: ask principal.ActionRead in Summary and this fails.
func TestTheFooterWarningIsWithheldFromASeatThatCannotFillTheDetails(t *testing.T) {
	reader := seat(map[string]principal.ObjectGrant{"installation_settings": {Read: true}})
	source, findings, err := Summary(reader, nil, nil, crmcontracts.HealthSource{Id: SourceID}, time.Time{}, func(int) crmcontracts.HealthFinding {
		t.Fatal("a withheld source built a finding")
		return crmcontracts.HealthFinding{}
	})
	if err != nil || len(findings) != 0 || source.Availability != crmcontracts.HealthSourceAvailabilityWithheld || source.ObservedAt != nil {
		t.Fatalf("a read-only seat got %+v %v %v, want the source withheld", source, findings, err)
	}
	if source.Permission != "installation_settings:update" {
		t.Errorf("the withheld source names %q, want the grant it needs", source.Permission)
	}
}
