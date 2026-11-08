// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARecordLinkNeedsAPageAnAddressAndAnID(t *testing.T) {
	id := ids.NewV7()
	if got := recordURL("https://crm.example", "deal", id); got == nil || *got != "https://crm.example/#/deals/"+id.String() {
		t.Fatalf("deal link = %v", got)
	}
	for _, tc := range []struct {
		name, base, entityType string
		id                     ids.UUID
	}{
		{"no public address", "", "deal", id},
		{"a type with no page", "https://crm.example", "offer", id},
		{"no id", "https://crm.example", "deal", ids.UUID{}},
	} {
		if got := recordURL(tc.base, tc.entityType, tc.id); got != nil {
			t.Errorf("%s: link = %s, want none", tc.name, *got)
		}
	}
}

func TestTheBaseAddressIsTrimmedOnce(t *testing.T) {
	d := (&Deliverer{}).WithPublicBaseURL("https://crm.example/")
	if d.publicBaseURL != "https://crm.example" {
		t.Fatalf("base = %q, want the trailing slash gone", d.publicBaseURL)
	}
}

func TestAPreviewSampleNamesARecordAndItsOwnerAndLinksIt(t *testing.T) {
	for _, tc := range []struct {
		eventType string
		wantName  bool
	}{
		{"deal.created", true},
		{"contact.created", false},
	} {
		raw, err := sampleEnvelope(tc.eventType)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Entity struct {
				Name      *string `json:"name"`
				OwnerName *string `json:"owner_name"`
				URL       *string `json:"url"`
			} `json:"entity"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		if env.Entity.URL == nil {
			t.Errorf("%s: the sample has no link, so a template using entity.url previews blank", tc.eventType)
		}
		if (env.Entity.Name != nil) != tc.wantName {
			t.Errorf("%s: name = %v, want present=%v", tc.eventType, env.Entity.Name, tc.wantName)
		}
		if (env.Entity.OwnerName != nil) != tc.wantName {
			t.Errorf("%s: owner name = %v, want present=%v", tc.eventType, env.Entity.OwnerName, tc.wantName)
		}
	}
}
