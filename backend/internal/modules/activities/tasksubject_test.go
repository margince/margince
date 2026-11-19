// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
)

// A task needs a name a reader can see. A subject of only invisible
// characters is as blank as an empty one, as it is for every other record.
func TestATaskSubjectOfOnlyInvisibleCharactersIsRefused(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "  ", "\u200b", " \u200b\u200d "} {
		if _, err := taskSubject(raw); !errors.As(err, new(*RequiredFieldError)) {
			t.Errorf("subject %q answered %v, want a required-field refusal", raw, err)
		}
	}
	if got, err := taskSubject("  Call back  "); err != nil || got != "Call back" {
		t.Fatalf("a real subject answered %q, %v", got, err)
	}
}
