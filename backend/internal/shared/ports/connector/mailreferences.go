// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"slices"
	"strings"
)

// MaxReplyReferences prevents sender-controlled headers from creating an unbounded edge set.
const MaxReplyReferences = 50

// MaxReplyReferenceBytes is the longest referenced id kept, the same bound the
// capture sink holds a thread key to (maxIndexedHeaderChars).
const MaxReplyReferenceBytes = 998

// OrderedMailReferences prioritizes In-Reply-To, then References from newest to oldest.
// The bound keeps nearest ancestors; self references and oversized IDs add no edge.
func OrderedMailReferences(references, inReplyTo, messageID string) []string {
	own := trimMailReference(strings.TrimSpace(messageID))
	refs := strings.Fields(references)
	candidates := make([]string, 0, len(refs)+1)
	candidates = append(candidates, strings.Fields(inReplyTo)...)
	for _, ref := range slices.Backward(refs) {
		candidates = append(candidates, ref)
	}
	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		id := trimMailReference(c)
		if id == "" || id == own || seen[id] || len(id) > MaxReplyReferenceBytes {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == MaxReplyReferences {
			break
		}
	}
	return out
}

func trimMailReference(id string) string { return strings.TrimSuffix(strings.TrimPrefix(id, "<"), ">") }
