// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestAChatShowsTheContactWhenANamePartOrTheNumberAgrees(t *testing.T) {
	names := []string{"Lena Hoffmann", "Lena", "Hoffmann"}
	phones := []string{"+491512345678"}
	for _, tc := range []struct {
		label, name, phone string
		shows              bool
	}{
		{"the full name", "Lena Hoffmann", "", true},
		{"the first name alone", "lena", "", true},
		{"the number in E.164 with spaces", "", "+49 151 2345678", true},
		{"the national form of the number", "", "0151 2345678", true},
		{"a stranger's number beside the right name", "Lena H.", "+44 20 7946 0000", true},
		{"someone else's name", "Jonas Reiter", "", false},
		{"someone else's number", "", "+44 20 7946 0000", false},
		{"an initial that matches nothing whole", "L. R.", "", false},
	} {
		if got := chatShowsContact(tc.name, tc.phone, names, phones); got != tc.shows {
			t.Errorf("%s: chatShowsContact = %v, want %v", tc.label, got, tc.shows)
		}
	}
}
