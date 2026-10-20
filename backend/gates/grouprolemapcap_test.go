// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

// The ceiling on the group to role map is read in three places, and they have
// to be one number.
//
// The contract publishes it as `maxProperties`, the server refuses a longer
// map on save, and the browser disables Add group at the same count.
//
// Move one and the other two are wrong in opposite directions. An admin either
// builds a map the server then refuses, or is stopped short of the ceiling it
// enforces.
//
// The contract is the one this gate reads from, because it is what both sides
// claim to mirror. The Go constant is imported rather than parsed, so its
// reading is the compiler's.
package gates

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/identity"
)

const frontendGroupGrants = "../frontend/src/features/sign-in/group-role-grants.tsx"

// tsGroupGrantCap reads the browser's own ceiling.
//
// The name is matched rather than the number. A second constant of the same
// value cannot then stand in for the one the screen reads.
var tsGroupGrantCap = regexp.MustCompile(`MAX_GROUP_GRANTS\s*=\s*(\d+)\b`)

func TestTheGroupRoleMapCapIsOneNumber(t *testing.T) {
	t.Parallel()
	published := contractMaxProperties(t, "UpdateInstallationSettingsRequest", "oidc_group_role_map")
	if identity.MaxGroupRoleMapEntries != published {
		t.Errorf(
			"the server refuses a map over %d entries, the contract publishes %d",
			identity.MaxGroupRoleMapEntries, published,
		)
	}

	source, err := os.ReadFile(frontendGroupGrants)
	if err != nil {
		t.Fatalf("reading the group grants screen: %v", err)
	}
	found := tsGroupGrantCap.FindStringSubmatch(string(source))
	if found == nil {
		t.Fatalf(
			"%s no longer declares MAX_GROUP_GRANTS — the browser's ceiling is somewhere this gate cannot see",
			frontendGroupGrants,
		)
	}
	inBrowser, err := strconv.Atoi(found[1])
	if err != nil {
		t.Fatalf("%s: %q is not a count", frontendGroupGrants, found[1])
	}
	if inBrowser != published {
		t.Errorf(
			"the browser stops at %d groups, the contract publishes %d",
			inBrowser, published,
		)
	}
}

// contractMaxProperties answers the ceiling the contract publishes for one
// object property.
//
// A property declaring none fails rather than answering zero. Read as zero,
// every other reading would disagree with it for the wrong reason.
func contractMaxProperties(t *testing.T, schema, property string) int {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MaxProperties *int `yaml:"maxProperties"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	shape, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("the contract has no schema %s", schema)
	}
	prop, ok := shape.Properties[property]
	if !ok {
		t.Fatalf("%s declares no property %s", schema, property)
	}
	if prop.MaxProperties == nil {
		t.Fatalf("%s.%s publishes no maxProperties, so neither side has a ceiling to mirror", schema, property)
	}
	return *prop.MaxProperties
}
