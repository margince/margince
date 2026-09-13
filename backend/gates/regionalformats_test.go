// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestRegionalFormatReadAndWriteContractsAgree(t *testing.T) {
	t.Parallel()
	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	read, ok := schemaBody(string(contract), "InstallationSettings")
	if !ok {
		t.Fatal("missing settings read schema")
	}
	write, ok := schemaBody(string(contract), "UpdateInstallationSettingsRequest")
	if !ok {
		t.Fatal("missing settings write schema")
	}
	// The store validates through the generated read enum. This mirror holds
	// the write contract to exactly the values that validator admits.
	for _, property := range []string{"date_format", "time_format"} {
		pattern := regexp.MustCompile(`(?m)^        ` + property + `:.*\n(?:          .*\n)*?          enum: \[([^\]]+)\]`)
		var values [][]string
		for _, schema := range []string{read, write} {
			match := pattern.FindStringSubmatch(schema)
			if match == nil {
				t.Fatalf("missing %s enum", property)
			}
			list := strings.Split(match[1], ",")
			for i := range list {
				list[i] = strings.TrimSpace(list[i])
			}
			sort.Strings(list)
			values = append(values, list)
		}
		if !reflect.DeepEqual(values[0], values[1]) {
			t.Errorf("%s read/write differ: %v", property, values)
		}
	}
}
