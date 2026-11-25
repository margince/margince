// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestNotesMailTitleIsTheQuotedTitleOfGeminisSubject(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		subject, want string
		ok            bool
	}{
		{`Notes: "Quarterly Review" Nov 3, 2026`, "quarterly review", true},
		{"Notes: \u201cKick-off  Review\u201d Nov 3, 2026", "kick-off review", true},
		{`Notes: "He said "hi" today" Nov 3, 2026`, `he said "hi" today`, true},
		{`Fwd: Notes: "Review" Nov 3, 2026`, "", false},
		{`Meeting records: "Review" Nov 3, 2026`, "", false},
		{`Review notes`, "", false},
		{`Notes: "" Nov 3, 2026`, "", false},
	} {
		got, ok := notesMailTitle(test.subject)
		if got != test.want || ok != test.ok {
			t.Errorf("notesMailTitle(%q) = %q, %v, want %q, %v", test.subject, got, ok, test.want, test.ok)
		}
	}
}
