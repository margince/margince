// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import "strings"

// Steering is what a caller says about one draft on every drafting route.
// It holds the ask they typed, and the draft already on their screen when the
// call is a rewrite. Both are trimmed, and empty means absent. A route therefore cannot
// read "   " as a first draft on one surface and as a rewrite on another.
type Steering struct {
	Intent    string
	RewriteOf string
}

// NewSteering reads the two optional request fields of crm.yaml's DraftSteering.
func NewSteering(intent, rewriteOf *string) Steering {
	var out Steering
	if intent != nil {
		out.Intent = strings.TrimSpace(*intent)
	}
	if rewriteOf != nil {
		out.RewriteOf = strings.TrimSpace(*rewriteOf)
	}
	return out
}
