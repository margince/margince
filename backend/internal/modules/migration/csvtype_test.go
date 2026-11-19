// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import "testing"

func TestTheTypeIsTheOneWhoseOwnColumnsTheHeaderMapsOntoMost(t *testing.T) {
	for _, tc := range []struct {
		name      string
		suggested map[string]map[string]string
		want      string
	}{
		{"the contact's own columns win", map[string]map[string]string{
			ObjectContact: {"Email": "email", "First Name": "first_name", "Last Name": "last_name"},
			ObjectLead:    {"Email": "email"},
		}, ObjectContact},
		{"columns every type maps choose nothing", map[string]map[string]string{
			ObjectContact: {"Email": "email"}, ObjectLead: {"Email": "email"},
		}, ""},
		{"a tie chooses nothing", map[string]map[string]string{
			ObjectContact: {"First Name": "first_name"}, ObjectLead: {"Status": "status"},
		}, ""},
		{"one own column outweighs any number of shared ones", map[string]map[string]string{
			ObjectContact: {"Email": "email", "Title": "title"},
			ObjectLead:    {"Email": "email", "Title": "title"},
			ObjectDeal:    {"Stage": "stage"},
		}, ObjectDeal},
		{"nothing mapped chooses nothing", map[string]map[string]string{ObjectContact: {}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ObjectOfMappings(tc.suggested); got != tc.want {
				t.Errorf("ObjectOfMappings = %q, want %q", got, tc.want)
			}
		})
	}
}
