// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A pinned read instant decides the SLA state on every lead read. The live
// clock is moved past the deadline, so a read that ignores the pin answers
// breached.
func TestEveryLeadReadJudgesSLAByThePinnedInstant(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	pinned := clocktest.Now(t).UTC().Truncate(time.Second)
	lead := e.seedLeadCreatedAt(t, "pinned@example.test", pinned.Add(-time.Hour))
	previousClock := leadSLAClock
	leadSLAClock = func() time.Time { return pinned.Add(2 * DefaultFirstResponseTarget) }
	t.Cleanup(func() { leadSLAClock = previousClock })
	ctx := WithReadInstant(e.ctx, pinned)
	within := crmcontracts.LeadSlaStateLeadSlaStateWithinTarget

	single, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if single.SlaState == nil || *single.SlaState != within {
		t.Errorf("the single read says %s, want %s at the pinned instant", slaStateOf(single.SlaState), within)
	}

	sorted := "created_at"
	withinFilter := crmcontracts.ListLeadsParamsSlaState(within)
	lists := map[string]ListLeadsInput{
		"the default queue":                    {},
		"a sorted list":                        {Sort: &sorted},
		"a sorted list filtered within target": {Sort: &sorted, SLAState: &withinFilter},
	}
	for name, in := range lists {
		rows, _, err := e.store.ListLeads(ctx, in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(rows) != 1 || rows[0].Id != single.Id {
			t.Errorf("%s lists %d leads, want the one lead", name, len(rows))
			continue
		}
		if rows[0].SlaState == nil || *rows[0].SlaState != within {
			t.Errorf("%s says %s, want %s at the pinned instant", name, slaStateOf(rows[0].SlaState), within)
		}
	}
}

func slaStateOf(state *crmcontracts.LeadSlaState) string {
	if state == nil {
		return "no state"
	}
	return string(*state)
}
