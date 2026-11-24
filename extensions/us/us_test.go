// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package us

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func TestTheUnitHoldsOnePackAndOneRuleSetForTheUnitedStates(t *testing.T) {
	e := New()
	if e.Name != "us" {
		t.Fatalf("Name = %q, want the unit's directory name us", e.Name)
	}
	if len(e.Jurisdictions) != 1 || e.Jurisdictions[0].Code() != "us" {
		t.Fatalf("jurisdiction packs %v, want one for us", e.Jurisdictions)
	}
	if len(e.Messaging) != 1 || e.Messaging[0].Jurisdiction != "us" {
		t.Fatalf("messaging rule sets %v, want one for us", e.Messaging)
	}
	if err := e.Messaging[0].Validate(); err != nil {
		t.Errorf("the rules would be refused at boot: %v", err)
	}
}

// The United States is an opt-out market with no route to name: advertising
// needs no consent, so there is no exception and no cap. It identifies an
// advertisement by a footer line (7704(a)(5)(A)(i)), not a subject prefix.
func TestTheUnitedStatesAllowsAdvertisingUntilARefusal(t *testing.T) {
	r := New().Messaging[0]
	if r.MarketingMode != messaging.MarketingOptOut {
		t.Errorf("marketing mode %q, want %q", r.MarketingMode, messaging.MarketingOptOut)
	}
	if r.ConsentOnly || len(r.MarketingExceptions) != 0 || r.FrequencyCap != nil {
		t.Errorf("rules %+v state a consent rule, exception or cap the Act does not", r)
	}
	if r.SubjectPrefix != "" {
		t.Errorf("subject prefix %q, want none: the footer line identifies the advertisement", r.SubjectPrefix)
	}
}

// CAN-SPAM 7704(a)(5)(A) asks every advertisement for a way to decline and the
// sender's postal address, which the controller's identity carries.
// It also asks that the message identify itself as an advertisement.
func TestAnAdvertisementOwesTheSenderAddressAndAWayToDecline(t *testing.T) {
	var kinds []messaging.DisclosureKind
	for _, d := range New().Messaging[0].Disclosures {
		if !d.MarketingOnly {
			t.Errorf("%s binds every message, want advertising only", d.Kind)
		}
		kinds = append(kinds, d.Kind)
	}
	slices.Sort(kinds)
	if want := []messaging.DisclosureKind{messaging.Advertisement, messaging.ControllerIdentity, messaging.ObjectionRoute}; !slices.Equal(kinds, want) {
		t.Errorf("disclosures %v, want %v", kinds, want)
	}
}

func TestTheUnitedStatesKeepsOneTaxRecordFloorAndNoCorrespondenceFloor(t *testing.T) {
	classes := New().Jurisdictions[0].Retention().Classes()
	if len(classes) != 1 || classes[0].Name != jurisdiction.AccountingRecords || classes[0].Law == "" ||
		classes[0].Keep.Years != 8 || classes[0].Anchor != jurisdiction.AnchorCalendarYearEnd {
		t.Errorf("retention %+v, want one accounting floor naming its law", classes)
	}
}
