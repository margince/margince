// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"testing"
)

// The money pair is judged jointly, and the half of that judgement which needs
// no trail is pinned here: an image stating BOTH halves is judged by the
// ordinary value comparison, and an image stating neither has no money to
// judge. Both return before any query, which the nil transaction proves — the
// trail branch, where one half is stated alone, is held by the integration
// test that writes a real later currency change.
func TestTheMoneyCouplingAsksTheTrailOnlyWhenOneHalfStandsAlone(t *testing.T) {
	for name, image := range map[string]string{
		"both halves stated": `{"amount_minor":1000,"currency":"EUR"}`,
		"no money at all":    `{"full_name":"Ada"}`,
	} {
		half, err := moneyMovedUnderIt(context.Background(), nil,
			AuditRow{After: json.RawMessage(image)})
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if half != "" {
			t.Errorf("%s: named %q, want no coupled half to judge", name, half)
		}
	}
}

// A later change to the CURRENCY supersedes a restore of the AMOUNT, and the
// refusal names the field the caller asked to put back rather than the sibling
// that moved.
func TestALaterCurrencyChangeSupersedesAnAmountRestore(t *testing.T) {
	got := reportedAs([]string{"currency"}, []string{"amount_minor"})
	if len(got) != 1 || got[0] != "amount_minor" {
		t.Errorf("reportedAs(currency moved, amount asked) = %v, want [amount_minor]", got)
	}
}

// A later write of an unrelated key supersedes nothing the caller asked about.
// Reporting it would refuse a restore the trail permits.
func TestAnUnrelatedLaterWriteSupersedesNothing(t *testing.T) {
	if got := reportedAs([]string{"title"}, []string{"full_name"}); len(got) != 0 {
		t.Errorf("reportedAs(title moved, full_name asked) = %v, want none", got)
	}
}

// A timestamp is one instant however the writer's zone spelled it, and
// anything that is not two timestamps is not an instant at all: a string that
// is not a time must stay different, or a changed name would read as unmoved.
func TestTwoSpellingsOfOneInstantAreTheSameValue(t *testing.T) {
	for name, tc := range map[string]struct {
		live, stated string
		want         bool
	}{
		"same instant, other zone": {`"2026-12-09T10:00:00+00:00"`, `"2026-12-09T17:00:00+07:00"`, true},
		"different instants":       {`"2026-12-09T10:00:00+00:00"`, `"2026-12-09T11:00:00+00:00"`, false},
		"plain text":               {`"Ada"`, `"Ada Lovelace"`, false},
		"live is not a string":     {`42`, `"2026-12-09T10:00:00Z"`, false},
		"stated is not a string":   {`"2026-12-09T10:00:00Z"`, `null`, false},
		"stated is not a time":     {`"2026-12-09T10:00:00Z"`, `"tomorrow"`, false},
	} {
		if got := sameInstant(json.RawMessage(tc.live), json.RawMessage(tc.stated)); got != tc.want {
			t.Errorf("%s: sameInstant(%s, %s) = %v, want %v", name, tc.live, tc.stated, got, tc.want)
		}
	}
}
