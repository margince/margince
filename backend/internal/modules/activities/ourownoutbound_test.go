// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Every statement that asks "is this our own message in this conversation"
// asks it the same way.
//
// Four of them spelled it out, agreeing clause for clause and differing only in
// what they added. Agreeing by coincidence is the state this replaces: the
// forged-thread-root guards reached one of them in a change that did not audit
// the other three, and nothing would have failed if they had been left behind.
//
// What this holds is that the shared text is shared. It does not run the SQL —
// the settlement and owed-verdict suites do that — and it cannot tell that a
// caller's own additions are right for it. Those are each written down beside
// the caller instead.

import (
	"strings"
	"testing"
)

// theseAskTheQuestion are the statements that identify our own outbound in a
// conversation, by the alias each gives the row it matches.
//
// Read from the built SQL rather than listed as file names: a fifth statement
// that composed the helper would be invisible to a list, and one that stopped
// composing it would still be on it.
func theseAskTheQuestion() map[string]string {
	return map[string]string{
		"repliedRequestsSQL":  repliedRequestsSQL,
		"newestOutboundSince": newestOutboundSince,
		"priorOutboundJoin":   priorOutboundJoin,
	}
}

func TestOurOwnOutboundIsAskedTheSameWayEverywhere(t *testing.T) {
	for name, sql := range theseAskTheQuestion() {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "counterparty_outbound_attested") {
				t.Fatalf("%s does not test the provider's own filing of the message as sent — "+
					"a typed References root would pass for a conversation", name)
			}
			if strings.Contains(sql, "thread_key IS NOT DISTINCT FROM") {
				t.Errorf("%s NULL-matches thread keys, which joins every threadless row to every "+
					"other", name)
			}
		})
	}
}

// The helper renders the clause set for whichever alias asks, and binds every
// clause to the anchor. A rendering that hard-coded one side would silently
// compare a row to itself.
func TestTheClauseSetBindsBothAliases(t *testing.T) {
	got := ourOutboundInThisThread("mine", "theirs")

	for _, want := range []string{
		"mine.thread_key = theirs.thread_key",
		"mine.kind = theirs.kind",
		"mine.channel_provider IS NOT DISTINCT FROM theirs.channel_provider",
		"mine.direction = 'outbound'",
		"mine.counterparty_email = theirs.counterparty_email",
		"mine.counterparty_outbound_attested",
		"mine.archived_at IS NULL",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the clause set does not carry %q", want)
		}
	}
	// No placeholder, so a caller's own $N numbering is untouched wherever this
	// lands in the statement. A helper that introduced one would renumber every
	// argument after it.
	if strings.Contains(got, "$") {
		t.Error("the clause set renders a placeholder, which shifts its caller's argument numbering")
	}
}
