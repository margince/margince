// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/capturepurgebind"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func previewCounts(t *testing.T, e *integration.Env, rule ids.UUID) capturepurgebind.Counts {
	t.Helper()
	seen := runPurge(t, e, e.Rep1, rule, true)
	return capturepurgebind.Counts{Destroyed: seen.Destroyed, Released: seen.Released, Anonymised: seen.Anonymised}
}

// Mail that arrives between the preview and the confirm is not destroyed: the
// caller confirmed a count, and a larger one was never shown to them.
func TestAPurgeBoundToAPreviewRefusesMailThatArrivedAfterIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before int
	}{{"zero then one", 0}, {"one then two", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			var first ids.UUID
			if tc.before == 1 {
				first = seedPurgeableMail(t, e, "late@example.test", "Erste", e.Rep1)
			}
			rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "late@example.test")
			seen := previewCounts(t, e, rule)
			if seen.Destroyed != tc.before {
				t.Fatalf("preview counted %d, want %d", seen.Destroyed, tc.before)
			}

			arrived := seedPurgeableMail(t, e, "late@example.test", "Spaet", e.Rep1)
			_, err := purgerFor(t, e).PurgeAsPreviewed(purgeCtx(e, e.Rep1), rule, seen)
			if !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("err = %v, want a conflict: the selection grew after the preview", err)
			}
			if activityBody(t, e, arrived) == "" {
				t.Fatal("mail the preview never counted was destroyed")
			}
			if first != ids.Nil && activityBody(t, e, first) == "" {
				t.Fatal("a refused purge still destroyed the previewed mail")
			}

			again := previewCounts(t, e, rule)
			if _, err := purgerFor(t, e).PurgeAsPreviewed(purgeCtx(e, e.Rep1), rule, again); err != nil {
				t.Fatalf("a purge bound to the fresh preview was refused: %v", err)
			}
		})
	}
}
