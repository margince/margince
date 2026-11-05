// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"
)

// publicEdgePrefixes are the paths the anonymous edges answer, each named by
// the constant its edge tests the request path against.
var publicEdgePrefixes = []string{
	publicFormsPrefix, publicConfirmPrefix, publicPreferencesPrefix, publicRecordPreviewPrefix,
	publicBookingPrefix, publicMeetingPrefix, publicProposalPrefix, publicDealRoomPrefix,
}

// The session middleware lets every /v1/public/ path through with no session
// and no workspace, trusting an edge to resolve its own token and throttle.
// A /public/ operation no edge claims falls through to the generated router and
// is served to anybody. So each one the contract declares must sit under a
// prefix an edge claims. The reverse holds too: a prefix no operation uses is
// an edge for a route that is gone.
func TestEveryPublicOperationOfTheContractIsAnsweredByAnEdge(t *testing.T) {
	operations, _ := passportOperations(t)
	claimed := map[string]int{}
	public := 0
	for _, op := range operations {
		if !strings.HasPrefix(op.route, "/v1/public/") {
			continue
		}
		public++
		index := slices.IndexFunc(publicEdgePrefixes, func(prefix string) bool { return strings.HasPrefix(op.route, prefix) })
		if index < 0 {
			t.Errorf("%s %s is a public operation no anonymous edge claims, so the session middleware "+
				"lets it through and nothing resolves its caller. Add its prefix to an edge, or move the route under a session",
				op.method, op.route)
			continue
		}
		claimed[publicEdgePrefixes[index]]++
	}
	if public == 0 {
		t.Fatal("the contract declares no /public/ operation, so this check compared nothing")
	}
	for _, prefix := range publicEdgePrefixes {
		if claimed[prefix] == 0 {
			t.Errorf("no operation of the contract sits under %s, so the edge that claims it answers a route that is gone", prefix)
		}
	}
}
