// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func sampleBody(overrides AnyMap) AnyMap {
	body := AnyMap{
		"kind": "email", "register": "email", "format": "text",
		"source_label": "A sample", "source_ref": "sample-ref",
		"content": "Plain prose that is the owner's own writing.",
	}
	maps.Copy(body, overrides)
	return body
}

func TestSampleWeightAbsentDefaultsToOneAndExplicitZeroIsKept(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/voice-profiles/" + createVoiceProfile(t, e).ID

	absent := ingest(t, e, base, sampleBody(AnyMap{"source_ref": "absent"}))
	if absent.Source.Weight != 1 {
		t.Errorf("absent weight stored as %v, want 1", absent.Source.Weight)
	}
	zero := ingest(t, e, base, sampleBody(AnyMap{"source_ref": "zero", "weight": 0}))
	if zero.Source.Weight != 0 {
		t.Errorf("explicit weight 0 stored as %v, want 0", zero.Source.Weight)
	}
}

func TestSampleLabelAndReferenceLimitsAreEnforced(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/voice-profiles/" + createVoiceProfile(t, e).ID

	for _, tc := range []struct {
		name      string
		overrides AnyMap
		field     string
	}{
		{"a 256-character label", AnyMap{"source_label": strings.Repeat("l", 256)}, "source_label"},
		{"a 513-character reference", AnyMap{"source_ref": strings.Repeat("r", 513)}, "source_ref"},
		{"an empty reference", AnyMap{"source_ref": ""}, "source_ref"},
		{"a blank reference", AnyMap{"source_ref": "   "}, "source_ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var refused problemWire
			if status := e.Call(t, "POST", base+"/sources", sampleBody(tc.overrides), nil, &refused); status != http.StatusUnprocessableEntity {
				t.Fatalf("status %d, want 422", status)
			}
			if len(refused.Details.Errors) != 1 || refused.Details.Errors[0].Field != tc.field {
				t.Errorf("errors %+v, want one naming %s", refused.Details.Errors, tc.field)
			}
		})
	}

	// Limits count characters: 255 two-byte letters are 510 bytes.
	ingest(t, e, base, sampleBody(AnyMap{"source_label": strings.Repeat("é", 255), "source_ref": strings.Repeat("é", 512)}))
}

func TestSampleReferenceIsTrimmedSoPaddedAndBareShareOneKey(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := "/v1/voice-profiles/" + createVoiceProfile(t, e).ID

	padded := ingest(t, e, base, sampleBody(AnyMap{"source_ref": " x "}))
	bare := ingest(t, e, base, sampleBody(AnyMap{"source_ref": "x"}))
	if padded.Source.ID != bare.Source.ID || bare.Source.SourceRef != "x" {
		t.Errorf("padded %q (%s) and bare %q (%s) are two sources", padded.Source.SourceRef, padded.Source.ID, bare.Source.SourceRef, bare.Source.ID)
	}
}
