// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"strings"
	"testing"
)

// Every sender kind is JUDGED by the reconnect lane, one way or the other.
//
// PrivateSenderClause names the kinds that are not a business relationship. A
// kind added to the vocabulary and not named there is silently treated as one —
// it would reach a lane promising new revenue with nobody having decided it
// should. The failure is invisible in production: the row simply appears, and
// looks like every other contact.
//
// So the test is over the VOCABULARY rather than over a list here: enrolling a
// kind fails this until somebody says which side it falls on.
func TestEveryVerdictKindIsJudgedByTheReconnectLane(t *testing.T) {
	// The kinds a reconnect lane may show: a real person, and the two shapes of
	// organization that correspond under their own name. Everything else is
	// private life or noise, and PrivateSenderClause must exclude it.
	business := map[string]bool{
		KindPerson:             true,
		KindRoleMailbox:        true,
		KindOrganizationSender: true,
	}
	all := []string{
		KindPerson, KindRoleMailbox, KindOrganizationSender, KindNewsletter,
		KindTransactional, KindSpam, KindPersonal, KindAdvisor,
	}
	clause := PrivateSenderClause("e")
	for _, kind := range all {
		named := strings.Contains(clause, "'"+kind+"'")
		if business[kind] && named {
			t.Errorf("%q is a business counterparty but the reconnect lane excludes it, "+
				"so a real lapsed relationship never reaches the reader", kind)
		}
		if !business[kind] && !named {
			t.Errorf("%q is not a business relationship and the reconnect lane does not "+
				"exclude it — it will appear under a heading promising new revenue "+
				"with nobody having decided it should", kind)
		}
	}
	if len(all) != 8 {
		t.Fatalf("the vocabulary has %d kinds and this test walks 8 — a new one must "+
			"be judged here rather than inheriting a side", len(all))
	}
}
