// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// The classification in isolation: each connection carries its WORST
// condition and no other, a parked mailbox raises nothing even with stale
// sidecar facts on it, and a healthy one stays silent.
func TestConnectionConcernCarriesTheWorstConditionOnly(t *testing.T) {
	auth := "auth"
	failed := &BackfillRun{Status: backfillStatusError}
	for _, tc := range []struct {
		name      string
		view      ConnectionView
		wantKind  string
		wantClass string
	}{
		{"healthy connected", ConnectionView{Status: "connected"}, "", ""},
		{"reauth outranks everything", ConnectionView{Status: statusReauthRequired, LastErrorClass: &auth, Backfill: failed}, ConcernReauthRequired, ""},
		{"error carries its class", ConnectionView{Status: statusError, LastErrorClass: &auth}, ConcernConnectionError, "auth"},
		{"error without a class", ConnectionView{Status: statusError}, ConcernConnectionError, ""},
		{"connected but failing", ConnectionView{Status: "connected", LastErrorClass: &auth}, ConcernSyncFailing, "auth"},
		{"failed history import", ConnectionView{Status: "connected", Backfill: failed}, ConcernBackfillFailed, ""},
		{"running import is not a failure", ConnectionView{Status: "connected", Backfill: &BackfillRun{Status: "running"}}, "", ""},
		{"parked mailbox stays quiet", ConnectionView{Status: statusDisconnected, LastErrorClass: &auth, Backfill: failed}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, class := connectionConcern(tc.view)
			if kind != tc.wantKind || class != tc.wantClass {
				t.Fatalf("connectionConcern = (%q, %q), want (%q, %q)", kind, class, tc.wantKind, tc.wantClass)
			}
		})
	}
}
