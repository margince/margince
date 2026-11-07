// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// The published document names exactly the views the policy table serves.
func TestTheViewsDocumentNamesEveryPolicyView(t *testing.T) {
	t.Parallel()
	views := readViews(agentPolicies)
	body, err := agents.NewReadViewsResource(views).ReadViewsDocument(context.Background())
	if err != nil {
		t.Fatalf("rendering the document: %v", err)
	}
	var doc struct {
		Views []struct {
			View string `json:"view"`
		} `json:"views"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("the document does not decode: %v", err)
	}
	listed := make([]string, 0, len(doc.Views))
	for _, entry := range doc.Views {
		listed = append(listed, entry.View)
	}
	served := make([]string, 0, len(views))
	for _, view := range views {
		served = append(served, view.Name)
	}
	slices.Sort(served)
	if len(served) == 0 || !slices.Equal(listed, served) {
		t.Errorf("the document names %v, the policy table serves %v", listed, served)
	}
}
