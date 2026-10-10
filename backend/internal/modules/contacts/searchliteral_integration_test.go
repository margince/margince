// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A search for `%` or `_` finds names holding that character, never every row.
func TestSearchTreatsPercentAndUnderscoreAsTypedText(t *testing.T) {
	e := setupCapturePrivacy(t)
	percent := e.seedContact(t, "100% Real", "pct@literal.example")
	fan := e.seedContact(t, "100X Fan", "fan@literal.example")
	under := e.seedContact(t, "snake_case Sam", "sam@literal.example")
	plain := e.seedContact(t, "Plain Pat", "pat@literal.example")

	cases := []struct {
		q       string
		want    ids.UUID
		notWant []ids.UUID
	}{
		{"100%", percent.UUID, []ids.UUID{fan.UUID, plain.UUID}},
		{"_", under.UUID, []ids.UUID{percent.UUID, fan.UUID, plain.UUID}},
		{"%", percent.UUID, []ids.UUID{fan.UUID, under.UUID, plain.UUID}},
	}
	for _, c := range cases {
		got := e.findContacts(t, c.q)
		if !holds(got, c.want) {
			t.Errorf("contact search %q misses the name that holds it", c.q)
		}
		for _, id := range c.notWant {
			if holds(got, id) {
				t.Errorf("contact search %q also returns a name without that character", c.q)
			}
		}
	}

	coPercent := e.seedAccount(t, "Half% Co", "half.example")
	coPlain := e.seedAccount(t, "Whole Co", "whole.example")
	got := e.findAccounts(t, "%")
	if !holds(got, coPercent.UUID) || holds(got, coPlain.UUID) {
		t.Errorf("company search %% returned %d rows; want only the name holding the character", len(got))
	}
}
