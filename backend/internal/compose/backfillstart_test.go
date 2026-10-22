// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// The import asked for at connect starts only if the callback holds the backfill
// engine. Asserted through the applied options, in every order a role can apply
// them. A struct literal that replaces connectorHandlers drops the field
// silently, and the callback then lands plain with nothing started.
func TestTheConnectImportIsWiredWhicheverCaptureOptionRunsFirst(t *testing.T) {
	const stateKey = "0123456789abcdef0123456789abcdef"
	gmail := WithGmailCapture(GmailConfig{
		ClientID: "id", ClientSecret: "sec", StateKey: stateKey, PublicBaseURL: "https://app",
	}, CaptureConfig{})
	graph := WithGraphCapture(GraphConfig{
		ClientID: "ms-id", ClientSecret: "ms-sec", StateKey: stateKey, PublicBaseURL: "https://app",
	})
	// The option only asks that a queue client exists; nothing is enqueued here.
	backfill := WithCaptureBackfill(&jobs.Runner{})

	for _, tc := range []struct {
		name  string
		order []Option
	}{
		{"gmail, then backfill, as cmd/api applies them", []Option{gmail, graph, backfill}},
		{"a Microsoft-only role", []Option{graph, backfill}},
		{"backfill before the gmail option replaces the handlers", []Option{graph, backfill, gmail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s Server
			WithKeyvault(fakeVault{})(&s, nil)
			for _, opt := range tc.order {
				opt(&s, nil)
			}
			if s.importer.estimate == nil || s.importer.begin == nil {
				t.Fatal("the connect callback holds no backfill engine, so an import asked for at connect never starts")
			}
		})
	}
}

// Without the backfill surface there is nothing to start, and the callback's
// own errImportUnwired is what says so.
func TestTheConnectImportStaysEmptyWithoutTheBackfillSurface(t *testing.T) {
	var s Server
	WithKeyvault(fakeVault{})(&s, nil)
	WithGmailCapture(GmailConfig{
		ClientID: "id", ClientSecret: "sec", StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app",
	}, CaptureConfig{})(&s, nil)

	if s.importer.estimate != nil || s.importer.begin != nil {
		t.Fatal("a role with no backfill queue was handed an import engine it cannot schedule")
	}
}
