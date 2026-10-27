// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A company's geocode status is spelled once in Go and once in the column's
// CHECK, and the two are the same set. A status the Go side writes and the
// CHECK refuses fails every lookup that reaches it. A status the CHECK admits
// and Go never names is a row no reader handles.
//
// It reads the schema as the migrations leave it, from the head catalog, so a
// later migration that widens the CHECK is what is compared. The address
// trigger stamps the stale status as a literal too, so its body is held to the
// same constant.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

// arrayLiteral is one quoted value in a CHECK's ARRAY[...] as the catalog
// renders it: 'ok'::text.
var arrayLiteral = regexp.MustCompile(`'([^']*)'::text`)

func TestTheGeocodeStatusesAreTheColumnsCheck(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(repoRoot, headCatalogPath))
	if err != nil {
		t.Fatalf("reading %s: %v", headCatalogPath, err)
	}
	catalog := string(raw)
	check := catalogLine(t, catalog, "public.company.company_geocode_status_check ")
	var admitted []string
	for _, m := range arrayLiteral.FindAllStringSubmatch(check, -1) {
		admitted = append(admitted, m[1])
	}
	named := slices.Clone(contacts.GeocodeStatuses())
	slices.Sort(admitted)
	slices.Sort(named)
	if !slices.Equal(admitted, named) {
		t.Errorf("company.geocode_status admits %v and contacts.GeocodeStatuses names %v. "+
			"Change both together, or a status one side writes is one the other refuses", admitted, named)
	}
	if stamp := "NEW.geocode_status := '" + contacts.GeocodeStale + "';"; !strings.Contains(catalog, stamp) {
		t.Errorf("the address trigger no longer stamps %q, so it and contacts.GeocodeStale have drifted", stamp)
	}
}

// catalogLine is the one catalog line that starts with prefix.
func catalogLine(t *testing.T, catalog, prefix string) string {
	t.Helper()
	for line := range strings.SplitSeq(catalog, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("%s holds no line starting %q", headCatalogPath, prefix)
	return ""
}
