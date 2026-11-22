// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// geminiWireRule versions how the Gemini adapters turn a request into what
// Gemini decodes under, which decides what a model can answer. wirerule_test.go
// fails an edit to that code that leaves it unbumped.
const geminiWireRule = "gemini-wire-4"

// WireRule is the version of provider's wire translation, so a record measured
// under another translation reads stale; empty for a provider that declares none.
func WireRule(provider string) string {
	d, _ := providerByName(provider)
	return d.wireRule
}
