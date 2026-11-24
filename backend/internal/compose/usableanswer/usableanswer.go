// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package usableanswer is the one rule every batched verdict site applies to a
// model's answers: drop the answer the site cannot use, keep the rest.
package usableanswer

import "github.com/margince/margince/backend/internal/shared/schema"

// ConfidenceInRange reports whether a model-reported score is a confidence at
// all. 90 and 9 both appear for "90%", so no rescaling is safe and a value
// outside [0,1] is read as no answer.
func ConfidenceInRange(c schema.Confidence) bool { return c >= 0 && c <= 1 }

// Keep drops the answers a site cannot use and keeps the rest of the reply.
// Unusable means an invalid label or verdict, or a confidence outside [0,1].
// Each site sends the dropped message through the solo retry that a
// below-floor answer takes. Run it after the id check, which counts a dropped
// answer as given.
func Keep[T any](results []T, usable func(T) bool) []T {
	kept := make([]T, 0, len(results))
	for _, r := range results {
		if usable(r) {
			kept = append(kept, r)
		}
	}
	return kept
}

// FirstFault names the first answer a site cannot use, or "" when each can be
// used. Certification grades a model on it: an answer the engine drops still
// counts against the model that wrote it.
func FirstFault[T any](results []T, fault func(T) string) string {
	for _, r := range results {
		if msg := fault(r); msg != "" {
			return msg
		}
	}
	return ""
}
