// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// What a snooze may wait for is spelled in four places, and all four must agree.
//
// values.ReopenConditions() is the Go set every writer builds from. The two
// tables' CHECK constraints are what the database will accept, and the
// contract's ReopenCondition enum is what a client may send. enumsync_test.go
// holds the Go set to both CHECKs; this file holds the wire to the Go set.
//
// Every side is DERIVED: the Go set from the function, the wire set from the
// YAML. Hard-coding the three values here would make this file a fifth copy
// rather than a check on the other four.

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestTheWireOffersExactlyTheConditionsGoCanWrite(t *testing.T) {
	t.Parallel()
	want := make([]string, 0, len(values.ReopenConditions()))
	for _, c := range values.ReopenConditions() {
		want = append(want, string(c))
	}
	slices.Sort(want)

	got := reopenContractEnum(t)
	if !slices.Equal(got, want) {
		t.Errorf("the contract's ReopenCondition enum is %v but values.ReopenConditions() is %v; "+
			"a value on the wire the server cannot store is a promise no writer keeps", got, want)
	}
}

// reopenContractEnum reads the shared schema's enum out of the contract.
func reopenContractEnum(t *testing.T) []string {
	t.Helper()
	body, err := gatekit.ReadFile(filepath.Join("api", "crm.yaml"))
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `yaml:"enum"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := unmarshalYAML(body, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	schema, ok := doc.Components.Schemas["ReopenCondition"]
	if !ok {
		t.Fatal("the contract has no ReopenCondition schema; the enum this gate holds is gone")
	}
	out := slices.Clone(schema.Enum)
	// The nullable spellings elsewhere carry a null member; this shared schema
	// does not, and a stray empty would compare unequal for a reason that has
	// nothing to do with drift.
	out = slices.DeleteFunc(out, func(v string) bool { return strings.TrimSpace(v) == "" })
	slices.Sort(out)
	return out
}
