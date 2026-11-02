// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The ids a reply names come back unbracketed, In-Reply-To first and then the
// References chain from its newest end, without the message's own id or a
// repeat — the order that keeps the nearest ancestors when the bound applies.
func TestReplyIDsListsTheNearestAncestorsFirst(t *testing.T) {
	got := connector.OrderedMailReferences("<root@x> <mid@x> <parent@x>", "<parent@x>", "<self@x>")
	want := []string{"parent@x", "mid@x", "root@x"}
	if !slices.Equal(got, want) {
		t.Fatalf("replyIDs = %v, want %v", got, want)
	}
}

// A header naming the message itself, an oversized id, or more ids than the
// bound contributes none of the excess: the header is the sender's text.
func TestReplyIDsBoundsWhatASenderCanMakeItStore(t *testing.T) {
	if got := connector.OrderedMailReferences("<self@x>", "", "<self@x>"); len(got) != 0 {
		t.Fatalf("a message naming itself links %v, want nothing", got)
	}
	huge := "<" + strings.Repeat("a", connector.MaxReplyReferenceBytes+1) + "@x>"
	if got := connector.OrderedMailReferences(huge, "", "<self@x>"); len(got) != 0 {
		t.Fatalf("an oversized id was kept: %d ids", len(got))
	}
	var many []string
	for i := range connector.MaxReplyReferences + 10 {
		many = append(many, "<id"+strings.Repeat("x", i)+"@x>")
	}
	if got := connector.OrderedMailReferences(strings.Join(many, " "), "", "<self@x>"); len(got) != connector.MaxReplyReferences {
		t.Fatalf("kept %d ids, want the bound %d", len(got), connector.MaxReplyReferences)
	}
}
