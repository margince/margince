// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgdossier

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The input carries the company's name for the rail and for nothing else: the
// model is not shown it, because a name grounds nothing, and the fingerprint
// does not hash it, because a rename is not a change in what the company IS
// and must not throw away every cached dossier.
func TestTheInputCarriesTheNameOutsideWhatTheModelAndTheCacheSee(t *testing.T) {
	in, err := BuildInput(context.Background(), stubFacts{in: Input{Name: "Acme Werkzeugbau"}}, ids.New[ids.OrganizationKind]())
	if err != nil {
		t.Fatalf("BuildInput: %v", err)
	}
	if in.Name != "Acme Werkzeugbau" {
		t.Fatalf("Name = %q, want the organization's display name", in.Name)
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "Acme") {
		t.Errorf("the name reached the JSON the model and the fingerprint read: %s", encoded)
	}
}
