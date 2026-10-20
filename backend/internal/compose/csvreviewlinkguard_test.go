// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestReviewedImportLinksNameTheEarlierRowTheyWillUse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		link   migration.ReviewLink
		target string
	}{
		{name: "source key", link: migration.ReviewLink{Object: migration.ObjectCompany, Key: "company-one", Pending: true}},
		{name: "canonical domain", link: migration.ReviewLink{Object: migration.ObjectCompany, Key: "example.test", Pending: true}},
		{name: "company name", link: migration.ReviewLink{Object: migration.ObjectCompany, Key: "  Example  Company ", ByName: true, Pending: true}},
		{name: "existing earlier row", link: migration.ReviewLink{Object: migration.ObjectCompany, Key: "company-one", Pending: true}, target: "019ae000-0000-7000-8000-000000000041"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := migration.ReviewPlan{Complete: true, Rows: []migration.ReviewRow{
				{Object: migration.ObjectCompany, ExternalID: "company-one", Outcome: "create", TargetID: tc.target, Values: map[string]json.RawMessage{
					fieldDisplayName: json.RawMessage(`"Example Company"`), fieldDomain: json.RawMessage(`"https://example.test"`),
				}},
				{Object: migration.ObjectContact, ExternalID: "person-one", Outcome: "create", Links: []migration.ReviewLink{tc.link}},
			}}
			settleReviewLinks(&plan)
			if !plan.Complete {
				t.Fatal("unambiguous row plan became incomplete")
			}
			link := plan.Rows[1].Links[0]
			if link.PendingKey != "company-one" || link.TargetID != tc.target || link.Pending != (tc.target == "") {
				t.Fatalf("link resolved to %+v, want the first company row", link)
			}
		})
	}
}

func TestReviewedImportRefusesAnAmbiguousPendingLink(t *testing.T) {
	plan := migration.ReviewPlan{Complete: true, Rows: []migration.ReviewRow{
		{Object: migration.ObjectCompany, ExternalID: "company-one", Outcome: "create", Values: map[string]json.RawMessage{fieldDisplayName: json.RawMessage(`"Example Company"`)}},
		{Object: migration.ObjectCompany, ExternalID: "company-two", Outcome: "create", Values: map[string]json.RawMessage{fieldDisplayName: json.RawMessage(`"Example Company"`)}},
		{Object: migration.ObjectContact, ExternalID: "person", Outcome: "create", Links: []migration.ReviewLink{{Object: migration.ObjectCompany, Key: "Example Company", ByName: true, Pending: true}}},
	}}
	settleReviewLinks(&plan)
	if plan.Complete {
		t.Fatal("an ambiguous company link remained approvable")
	}
}
