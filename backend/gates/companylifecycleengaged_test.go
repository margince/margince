// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every company lifecycle the contract publishes is judged engaged or not engaged, once.
//
// Mail reminders, ghosted-thread nudges and the offer to move an account to
// former customer all ask values.EngagedLifecycles. A stage the contract gains
// and that list never judged would drop out of all three with no error, so the
// judged list must equal each of the contract's lifecycle enums. The browser's
// picker list is a declared mirror of it, in the same wire order.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// companyLifecycleEnums names the generated types of the four inline
// lifecycle enums in crm.yaml.
var companyLifecycleEnums = []string{
	"CompanyLifecycle",
	"Company360StateStripAccountLifecycle",
	"UpdateCompanyRequestLifecycle",
	"ListCompaniesParamsLifecycle",
}

const frontendLifecycleList = "../frontend/src/format/companylifecycle.ts"

func TestEveryCompanyLifecycleIsJudgedEngagedOrNot(t *testing.T) {
	t.Parallel()
	judged := values.ClassifiedLifecycles()
	if dup := firstDuplicate(judged); dup != "" {
		t.Fatalf("values judges the lifecycle %q twice; keep one entry", dup)
	}
	want := slices.Sorted(slices.Values(judged))
	members := contractEnumValues(t)
	for _, enum := range companyLifecycleEnums {
		got := slices.Sorted(slices.Values(members[enum]))
		if !slices.Equal(got, want) {
			t.Errorf("crm.yaml's %s holds %v, and values judges %v. "+
				"Add the stage to companyLifecycles in internal/shared/kernel/values/lifecycle.go "+
				"and say whether it is engaged, or add it to every lifecycle enum in crm.yaml", enum, got, want)
		}
	}
	for _, engaged := range values.EngagedLifecycles() {
		if !slices.Contains(judged, engaged) {
			t.Errorf("%q is engaged but not a judged lifecycle", engaged)
		}
	}
}

func TestTheBrowserOffersTheLifecyclesInWireOrder(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendLifecycleList)
	if err != nil {
		t.Fatalf("reading the browser's lifecycle list: %v", err)
	}
	list := regexp.MustCompile(`(?s)export const LIFECYCLE_OPTIONS = \[(.*?)\]`).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s declares no LIFECYCLE_OPTIONS array; the mirror this gate holds has moved", frontendLifecycleList)
	}
	var offered []string
	for _, quoted := range regexp.MustCompile(`"([a-z_]+)"`).FindAllSubmatch(list[1], -1) {
		offered = append(offered, string(quoted[1]))
	}
	if judged := values.ClassifiedLifecycles(); !slices.Equal(offered, judged) {
		t.Errorf("the browser offers %v, and the server's wire order is %v; make LIFECYCLE_OPTIONS match", offered, judged)
	}
}

// contractEnumValues reads every string constant of the generated contract,
// keyed by its enum type.
func contractEnumValues(t *testing.T) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(contractsPkgDir)
	if err != nil {
		t.Fatalf("reading %s: %v", contractsPkgDir, err)
	}
	out := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(contractsPkgDir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		collectEnumValues(t, file, out)
	}
	for _, enum := range companyLifecycleEnums {
		if len(out[enum]) == 0 {
			t.Fatalf("no constants of %s under %s; the enum was renamed, so update companyLifecycleEnums", enum, contractsPkgDir)
		}
	}
	return out
}

func collectEnumValues(t *testing.T, file *ast.File, out map[string][]string) {
	t.Helper()
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			enum, ok := vs.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for _, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("reading the constant %s: %v", lit.Value, err)
				}
				out[enum.Name] = append(out[enum.Name], text)
			}
		}
	}
}

func firstDuplicate(list []string) string {
	seen := map[string]bool{}
	for _, item := range list {
		if seen[item] {
			return item
		}
		seen[item] = true
	}
	return ""
}
