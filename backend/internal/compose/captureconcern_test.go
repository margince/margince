// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestConcernKindsMatchTheContract(t *testing.T) {
	for _, kind := range capture.ConcernKinds {
		if !crmcontracts.CaptureConcernKind(kind).Valid() {
			t.Fatalf("unpublished concern %q", kind)
		}
	}
	doc, err := openapi3.NewLoader().LoadFromFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	kinds := doc.Components.Schemas["CaptureConcernKind"].Value.Enum
	if len(kinds) == 0 {
		t.Fatal("empty concern vocabulary")
	}
	for _, raw := range kinds {
		kind, ok := raw.(string)
		if !ok {
			t.Fatalf("non-string concern %v", raw)
		}
		if !slices.Contains(capture.ConcernKinds, kind) {
			t.Fatalf("unclassified concern %q", kind)
		}
	}
}

func TestConcernPriorityMatchesTheBanner(t *testing.T) {
	source, err := os.ReadFile("../../../frontend/src/lib/captureconcern.ts")
	if err != nil {
		t.Fatal(err)
	}
	entries := regexp.MustCompile(`(?m)^\s*([a-z_]+): "captureConcern\.`).FindAllSubmatch(source, -1)
	kinds := make([]string, 0, len(entries))
	for _, entry := range entries {
		kinds = append(kinds, string(entry[1]))
	}
	if !slices.Equal(kinds, capture.ConcernKinds) {
		t.Fatalf("banner priority %v differs from server %v", kinds, capture.ConcernKinds)
	}
}
