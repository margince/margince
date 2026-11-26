// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// A hand-logged mail's own thread holds only what its logger typed, so the
// transmit phase never reads it as the subject writing in. The answer comes
// before any query, which is why a nil transaction is enough here.
func TestAHandLoggedThreadIsNeverTheSubjectWritingIn(t *testing.T) {
	key := correspondence.HandLoggedThreadPrefix + "0193a1c2-0000-7000-8000-000000000001"
	found, err := wroteIntoThread(context.Background(), nil, key, subjectRef{})
	if err != nil {
		t.Fatalf("wroteIntoThread(%q): %v", key, err)
	}
	if found {
		t.Fatalf("wroteIntoThread(%q) found the subject writing in, want a hand-logged thread to prove nothing", key)
	}
}
