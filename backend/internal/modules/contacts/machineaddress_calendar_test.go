// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A calendar address is refused however its domain is spelled, as capture
// reads it.
func TestACalendarAddressIsRefusedInEverySpelling(t *testing.T) {
	for _, address := range []string{
		"c_0b8d@group.calendar.google.com",
		"C_0B8D@GROUP.CALENDAR.GOOGLE.COM",
		"c_0b8d@group.calendar.google.com.",
		"c_0b8d@eu.group.calendar.google.com",
		"room@resource.calendar.google.com",
	} {
		if refuseMachineAddress(address) == nil {
			t.Errorf("%q was accepted as a human's address", address)
		}
	}
	if err := refuseMachineAddress("anna@calendar.example"); err != nil {
		t.Errorf("a human's address was refused: %v", err)
	}
}
