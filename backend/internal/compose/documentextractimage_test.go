// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// A HEIC scan takes the card photo's image policy. A lane that reads JPEG and
// not HEIC still reads it, as JPEG.
func TestAHEICDocumentIsConvertedForALaneThatReadsJPEG(t *testing.T) {
	heic, err := os.ReadFile("../platform/imagenorm/testdata/card.heic")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeReadStore{meta: textAttachment("image/heic"), body: string(heic)}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(allFour()), carriage: []string{"image/jpeg"}}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	calls := brain.Calls()
	if len(calls) != 1 {
		t.Fatalf("model calls = %d, want 1; outcome %+v", len(calls), store.outcome)
	}
	if payload := string(calls[0].Payload); !strings.Contains(payload, "image/jpeg") || strings.Contains(payload, `"image/heic"`) {
		t.Errorf("the scan did not ride as a JPEG part:\n%.400s", payload)
	}
}

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
