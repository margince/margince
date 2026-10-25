// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// An image the lane cannot read fails the reading and names the type, the
// same answer the card photo gives.
func TestAnImageDocumentALaneCannotReadNamesTheType(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("image/gif"), body: "GIF89a"}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: []string{"image/jpeg"}}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome == nil || store.outcome.Status != activities.ExtractionReadFailed ||
		!strings.Contains(store.outcome.Detail, "image/gif") {
		t.Fatalf("outcome = %+v, want a failed reading naming image/gif", store.outcome)
	}
	if n := len(brain.Calls()); n != 0 {
		t.Errorf("made %d model call(s) for an image the lane cannot read", n)
	}
}
