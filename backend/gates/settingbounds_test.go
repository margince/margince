// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A bounded setting's range is one fact stated in three places: the entry's
// validator, which refuses past it; api/crm.yaml's minimum and maximum, which
// a caller reads; and the screen that refuses an out-of-range value in the box
// before the request. Two that disagree send a reader a refusal for a value the
// other side accepts.
//
// Every pair is DERIVED rather than listed. A contract property is held to the
// registered setting whose name it carries (`<module>.<name>` → `name`), and a
// screen states its mirror in the one shape `{ min: N, max: M }` keyed by the
// wire property, which is what this scan reads.

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// boundedPropertyFloor guards against a vacuous pass: the contract names this
// many bounded properties after a registered setting today, and a walk that
// matched none — a renamed schema root, a changed key shape — still trips it.
const boundedPropertyFloor = 8

// frontendBoundMirror is how a screen spells a bound it refuses before the
// request.
var frontendBoundMirror = regexp.MustCompile(`\b([a-z][a-z0-9_]*): \{ min: ([0-9_]+), max: ([0-9_]+) \}`)

type contractBound struct {
	schema   string
	min, max int
}

// boundedContractProperties maps a property name to every schema that states
// a minimum and maximum for it.
func boundedContractProperties(t *testing.T) map[string][]contractBound {
	t.Helper()
	schemas := descendContract(t, loadContractDocument(t), "components", "schemas")
	out := map[string][]contractBound{}
	for _, schemaName := range slices.Sorted(maps.Keys(schemas)) {
		schema, _ := schemas[schemaName].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for name, raw := range properties {
			property, _ := raw.(map[string]any)
			lowest, hasMin := property["minimum"].(int)
			highest, hasMax := property["maximum"].(int)
			if hasMin && hasMax {
				out[name] = append(out[name], contractBound{schema: schemaName, min: lowest, max: highest})
			}
		}
	}
	return out
}

func TestEveryContractBoundMatchesTheSettingThatEnforcesIt(t *testing.T) {
	t.Parallel()
	bounded := boundedContractProperties(t)
	checked := 0
	for _, setting := range compose.SettingsCatalogForTest() {
		_, name, _ := strings.Cut(setting.Key, ".")
		for _, bound := range bounded[name] {
			checked++
			for _, probe := range []struct {
				value int
				admit bool
			}{{bound.min, true}, {bound.max, true}, {bound.min - 1, false}, {bound.max + 1, false}} {
				err := setting.Admits(json.RawMessage(strconv.Itoa(probe.value)))
				if probe.admit && err != nil {
					t.Errorf("%s.%s states %d..%d, but %s refuses %d: %v",
						bound.schema, name, bound.min, bound.max, setting.Key, probe.value, err)
				}
				if !probe.admit && err == nil {
					t.Errorf("%s.%s states %d..%d, but %s admits %d",
						bound.schema, name, bound.min, bound.max, setting.Key, probe.value)
				}
			}
		}
	}
	if checked < boundedPropertyFloor {
		t.Fatalf("held %d bounded contract properties to their settings, want at least %d — the walk stopped finding them", checked, boundedPropertyFloor)
	}
}

func TestEveryScreenBoundMirrorsTheContract(t *testing.T) {
	t.Parallel()
	bounded := boundedContractProperties(t)
	mirrors := 0
	err := filepath.WalkDir("../frontend/src", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return err
		}
		if name := d.Name(); !strings.HasSuffix(name, ".tsx") && !strings.HasSuffix(name, ".ts") || strings.Contains(name, ".test.") {
			return nil
		}
		source, readErr := os.ReadFile(path) // #nosec G304,G122 -- a walk over this repository's own frontend tree
		if readErr != nil {
			return readErr
		}
		for _, match := range frontendBoundMirror.FindAllStringSubmatch(string(source), -1) {
			mirrors++
			if problem := mirrorProblem(match[1], match[2], match[3], bounded[match[1]]); problem != "" {
				t.Errorf("%s: %s", path, problem)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the frontend: %v", err)
	}
	if mirrors == 0 {
		t.Fatal("found no `name: { min: N, max: M }` mirror in the frontend; the scan reads nothing")
	}
}

// mirrorProblem says what is wrong with one screen-side bound, or "" when it
// agrees with every schema that bounds the same property.
func mirrorProblem(name, rawMin, rawMax string, stated []contractBound) string {
	if len(stated) == 0 {
		return fmt.Sprintf("%s mirrors a bound the contract does not state; bound it in api/crm.yaml or drop the mirror", name)
	}
	lowest, minErr := strconv.Atoi(strings.ReplaceAll(rawMin, "_", ""))
	highest, maxErr := strconv.Atoi(strings.ReplaceAll(rawMax, "_", ""))
	if minErr != nil || maxErr != nil {
		return fmt.Sprintf("%s states a bound that is not a whole number", name)
	}
	for _, bound := range stated {
		if bound.min != lowest || bound.max != highest {
			return fmt.Sprintf("%s refuses outside %d..%d, but %s states %d..%d", name, lowest, highest, bound.schema, bound.min, bound.max)
		}
	}
	return ""
}
