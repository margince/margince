// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// threadTopicCases are the subjects a conversation boundary compares, and the
// topic each reads as. The SQL spelling is held to the same table.
var threadTopicCases = []struct{ subject, topic string }{
	{"[ERP-27] kickoff", "kickoff"},
	{"Re: kickoff", "kickoff"},
	{"RE: [erp-27] Kickoff", "kickoff"},
	{"Kickoff [ERP-27] notes", "kickoff notes"},
	{"[ERP 27] kickoff", "[erp 27] kickoff"},
	{"[2026] budget", "[2026] budget"},
	{"Product materials", "product materials"},
	{"", ""},
}

// A reply that keeps or drops its project tag is on the same topic. A bracket
// that is not a key's shape stays, as the key tokenizer reads it.
func TestAThreadTopicLeavesOutProjectKeyTags(t *testing.T) {
	t.Parallel()
	for _, c := range threadTopicCases {
		if got := threadTopic(c.subject); got != c.topic {
			t.Errorf("threadTopic(%q) = %q, want %q", c.subject, got, c.topic)
		}
	}
}
