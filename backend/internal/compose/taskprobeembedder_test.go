// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The embed lane a DB-less debug loop ranks with: what it refuses, and what it
// reports about the binding it bound.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A model spec that names no binding is refused before anything is built or
// paid for.
func TestTheProbeRefusesAModelSpecThatNamesNoBinding(t *testing.T) {
	for _, spec := range []string{"", "justamodel", ":model", "provider:"} {
		if _, _, err := TaskProbeEmbedder(spec); err == nil {
			t.Errorf("--model %q is not provider:model and must be refused", spec)
		}
	}
}

// The identity and the width the probe ranks by are the BINDING's, not
// something this file chose: the question and the corpus have to be embedded by
// one lane at one width or the ranking measures nothing.
func TestTheProbeEmbedderReportsTheBindingsOwnIdentity(t *testing.T) {
	embedder, banner, err := TaskProbeEmbedder("fake:fake-embed")
	if err != nil {
		t.Fatalf("building the embed lane: %v", err)
	}
	if !strings.Contains(banner, "fake:fake-embed") {
		t.Errorf("banner = %q, want it to name the binding a run was made under", banner)
	}
	identity, dims := embedder.EmbedIdentity()
	if !strings.HasPrefix(identity, "fake/fake-embed@") {
		t.Errorf("identity = %q, want the bound provider and model", identity)
	}
	if dims <= 0 {
		t.Fatalf("width = %d, and a cache keyed on it would serve vectors of any shape", dims)
	}
	// The width in the identity is the width vectors come back at — one binding
	// reported two ways is a cache that serves the wrong space.
	if !strings.HasSuffix(identity, fmt.Sprintf("@%d", dims)) {
		t.Errorf("identity %q does not name the width %d it embeds at", identity, dims)
	}
	res, err := embedder.Embed(principal.WithWorkspaceID(t.Context(), ids.NewV7()),
		model.EmbedRequest{Inputs: []string{"how can I create a project"}, Dimensions: dims})
	if err != nil {
		t.Fatalf("the bound lane must embed: %v", err)
	}
	if len(res.Vectors) != 1 || res.Dims != dims {
		t.Fatalf("the lane answered %d vector(s) of width %d, want 1×%d", len(res.Vectors), res.Dims, dims)
	}
}
