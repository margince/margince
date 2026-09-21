// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The readiness message, which is the whole of what an operator gets.
//
// The probe's verdict is proved against a real database in
// integration/schemareadiness_integration_test.go; what cannot be proved there
// without contriving several broken namespaces at once is that a database
// behind on MORE than one reports all of them. A message naming only the first
// hands back one defect per scrape.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

func TestTheReadinessMessageNamesEveryNamespaceThatIsBehind(t *testing.T) {
	t.Parallel()

	got := describeShortfalls([]dbmigrate.Shortfall{
		{Namespace: "core", Missing: []string{"1789000001", "1789000002"}},
		{Namespace: "ext_crm_demo", Missing: []string{"0001"}, Untracked: true},
	})

	for _, want := range []string{"core", "1789000001", "1789000002", "ext_crm_demo"} {
		if !strings.Contains(got, want) {
			t.Errorf("the readiness message does not name %q: %s", want, got)
		}
	}
}

func TestANamespaceThatWasNeverAppliedReadsDifferentlyFromOneThatIsBehind(t *testing.T) {
	t.Parallel()

	// The two are different operator actions — one is "migrate", the other is
	// usually "this is not the database you think it is" — and a message that
	// could not tell them apart sends an operator to the wrong one.
	behind := describeShortfalls([]dbmigrate.Shortfall{{Namespace: "ext_crm_demo", Missing: []string{"0002"}}})
	never := describeShortfalls([]dbmigrate.Shortfall{{Namespace: "ext_crm_demo", Missing: []string{"0001", "0002"}, Untracked: true}})

	if behind == never {
		t.Fatalf("a namespace behind by a version and one never applied here read identically: %s", behind)
	}
	if !strings.Contains(never, "never applied") {
		t.Errorf("a namespace with no tracking table does not say so: %s", never)
	}
	if strings.Contains(behind, "never applied") {
		t.Errorf("a namespace that is merely behind is reported as never applied: %s", behind)
	}
}
