// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

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
	in, err := BuildInput(context.Background(), stubFacts{in: Input{Name: "Acme Werkzeugbau"}}, ids.New[ids.CompanyKind]())
	if err != nil {
		t.Fatalf("BuildInput: %v", err)
	}
	if in.Name != "Acme Werkzeugbau" {
		t.Fatalf("Name = %q, want the company's display name", in.Name)
	}
	fingerprinted, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for reader, encoded := range map[string]string{"fingerprint": string(fingerprinted), "model": encodeInput(in)} {
		if strings.Contains(encoded, "Acme") {
			t.Errorf("the name reached the JSON the %s reads: %s", reader, encoded)
		}
	}
}

// The model cites a record by the key it read it under, so every key of the
// JSON it reads is a citable entity type and each record kind sits under its
// own. A key spelled any other way is a type the grounding filter never matches.
func TestTheModelReadsEachRecordUnderTheEntityTypeItCites(t *testing.T) {
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encodeInput(Input{})), &keyed); err != nil {
		t.Fatalf("the model input is not a JSON object: %v", err)
	}
	want := map[string]bool{citeProfileField: true, citeFact: true}
	for key := range keyed {
		if !want[key] {
			t.Errorf("the model input lists records under %q, which is no entity type a citation may name", key)
		}
		delete(want, key)
	}
	for missing := range want {
		t.Errorf("the model input has no %q key, so it cannot tell which records are that type", missing)
	}
}
