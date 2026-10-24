// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A provenance tag names the agent behind a proposal or a change by its words,
// never by its principal id. Each named agent principal needs one
// `agentJob.<name>` message, and a message no principal earns is dead.
//
// An extension dispatcher (`"agent:" + decl.DispatcherKind()`) is composed at
// run time, so it is read from the units' own job declarations instead. The
// core catalogue cannot know a unit's jobs, so the frontend names those from
// their kind, and a core message for one is refused.
//
// H2: a passport uuid is no name any census can read.

import (
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/pkg/extension"
)

// tsAgentLabelKey reads one `"agentJob.<name>":` key out of the English
// catalogue, after tsComment has stripped its comments.
var tsAgentLabelKey = regexp.MustCompile(`"agentJob\.([a-z0-9_]+)"\s*:`)

// agentPrincipalText is a string that is a whole named agent principal.
var agentPrincipalText = regexp.MustCompile(`^agent:[a-z][a-z0-9_-]*$`)

// agentPrefixText is the bare prefix a composed principal starts from.
const agentPrefixText = "agent:"

func TestEveryAgentPrincipalHasAFrontendLabel(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendCatalogue)
	if err != nil {
		t.Fatalf("reading the frontend catalogue: %v", err)
	}
	labelled := map[string]bool{}
	for _, m := range tsAgentLabelKey.FindAllStringSubmatch(tsComment.ReplaceAllString(string(source), " "), -1) {
		labelled[m[1]] = true
	}

	bound := boundAgentPrincipals(t)
	if len(bound) == 0 || len(labelled) == 0 {
		t.Fatalf("read %d agent principals and %d labels — a census that reads nothing agrees with everything", len(bound), len(labelled))
	}
	for _, name := range sortedKeys(bound) {
		if !labelled[name] {
			t.Errorf("%s stamps agent principal %q, and %s has no \"agentJob.%s\" message (add it to de.ts and vi.ts too), so its provenance tag reads only \"Automated by an agent\"", bound[name], name, frontendCatalogue, name)
		}
	}
	for _, name := range sortedKeys(labelled) {
		if strings.HasPrefix(name, extension.NamespacePrefix) {
			t.Errorf("%s labels agentJob.%s, an extension unit's principal; the core catalogue cannot follow a unit's jobs and the tag names it from its kind — delete the message", frontendCatalogue, name)
			continue
		}
		if _, ok := bound[name]; !ok {
			t.Errorf("%s labels agentJob.%s, which no agent principal in the backend stamps — delete the message or stamp the principal", frontendCatalogue, name)
		}
	}
}

// boundAgentPrincipals reads the named agent principals product code can
// stamp, keyed by label key, valued by a file that names it. It reads every
// literal that is a whole `agent:<name>`, and every constant that folds to one
// (`"agent:" + approvalsubject.KindCounterparty`).
func boundAgentPrincipals(t *testing.T) map[string]string {
	t.Helper()
	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: namesAnAgentPrincipal,
	}.Files(t)
	found := map[string]string{}
	consts := packageConsts{t: t, byDir: map[string]map[string]string{}}
	for _, parsed := range files {
		if builtOnlyForTests(parsed.File) {
			continue
		}
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BasicLit:
				if text, err := strconv.Unquote(node.Value); err == nil && agentPrincipalText.MatchString(text) {
					found[agentLabelKey(text)] = parsed.Path
				}
			case *ast.ValueSpec:
				for _, value := range node.Values {
					if text, ok := foldConstant(consts, parsed, value); ok && agentPrincipalText.MatchString(text) {
						found[agentLabelKey(text)] = parsed.Path
					}
				}
			}
			return true
		})
	}
	return found
}

// agentLabelKey is the frontend's reading of an agent principal (jobLabel in
// provenance.tsx): the prefix dropped, and hyphens read as underscores.
func agentLabelKey(principalName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(principalName, agentPrefixText), "-", "_")
}

// namesAnAgentPrincipal holds a file with a whole agent principal or the bare
// prefix one is composed from.
func namesAnAgentPrincipal(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok {
			text, err := strconv.Unquote(lit.Value)
			found = found || (err == nil && (text == agentPrefixText || agentPrincipalText.MatchString(text)))
		}
		return !found
	})
	return found
}

// foldConstant reads a constant's value through gatekit.StringExpr, with the
// package's own constants and every module constant it names by qualifier
// (`"agent:" + approvalsubject.KindCounterparty`) in the table.
func foldConstant(consts packageConsts, parsed gatekit.ParsedFile, expr ast.Expr) (string, bool) {
	table := maps.Clone(consts.of(packageDir(parsed)))
	return gatekit.StringExpr(qualifiedAsIdents(consts, parsed, expr, table), table, gatekit.FoldStrict)
}

// qualifiedAsIdents copies a concatenation with each readable `pkg.Name`
// turned into an identifier spelled "pkg.Name", whose value it adds to table.
// The dot keeps the name apart from any local one. A selector this cannot read
// stays as it is, and the strict fold then refuses the whole expression.
func qualifiedAsIdents(consts packageConsts, parsed gatekit.ParsedFile, expr ast.Expr, table map[string]string) ast.Expr {
	switch node := expr.(type) {
	case *ast.SelectorExpr:
		pkg, isIdent := node.X.(*ast.Ident)
		if text, ok := consts.read(parsed, node); ok && isIdent {
			name := pkg.Name + "." + node.Sel.Name
			table[name] = text
			return ast.NewIdent(name)
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			X:  qualifiedAsIdents(consts, parsed, node.X, table),
			Op: node.Op,
			Y:  qualifiedAsIdents(consts, parsed, node.Y, table),
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{X: qualifiedAsIdents(consts, parsed, node.X, table)}
	case *ast.CallExpr:
		if name, ok := node.Fun.(*ast.Ident); ok && name.Name == "string" && len(node.Args) == 1 {
			return &ast.CallExpr{Fun: node.Fun, Args: []ast.Expr{qualifiedAsIdents(consts, parsed, node.Args[0], table)}}
		}
	}
	return expr
}

// builtOnlyForTests reports a file behind the integration build tag, whose
// principals a test harness stamps and no shipped binary does.
func builtOnlyForTests(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build") && strings.Contains(comment.Text, "integration") {
				return true
			}
		}
	}
	return false
}

const (
	extensionUnitsDir = "../extensions"
	provenanceSource  = "../frontend/src/design-system/provenance.tsx"
)

// tsExtensionJobPrefix reads the prefix the frontend names extension agents by.
var tsExtensionJobPrefix = regexp.MustCompile(`const EXTENSION_JOB_PREFIX = "([^"]*)"`)

// Every extension dispatcher an `agent:` principal is composed from is read
// from its unit's api/jobs.yaml and cross-checked against the Jobs the unit's
// Go code declares. Each must carry the prefix the frontend names it by.
func TestEveryExtensionAgentIsNamedFromItsKind(t *testing.T) {
	t.Parallel()
	kinds := extensionDispatcherKinds(t, extensionUnitsDir)
	declared := extensionDeclaredJobs(t, extensionUnitsDir)
	if len(declared) == 0 {
		t.Fatalf("read no extension.Job declaration under %s — a census that reads nothing agrees with everything", extensionUnitsDir)
	}
	for _, kind := range sortedKeys(declared) {
		if _, ok := kinds[kind]; !ok {
			t.Errorf("%s declares a job whose dispatcher kind %q no api/jobs.yaml dispatcher spells; the census of extension agents is short", declared[kind], kind)
		}
	}
	for _, kind := range sortedKeys(kinds) {
		if !strings.HasPrefix(kind, extension.NamespacePrefix) {
			t.Errorf("%s declares dispatcher %q outside the %q namespace, so the provenance tag cannot tell it is an extension's", kinds[kind], kind, extension.NamespacePrefix)
		}
	}
	source, err := os.ReadFile(provenanceSource)
	if err != nil {
		t.Fatalf("reading %s: %v", provenanceSource, err)
	}
	prefix := tsExtensionJobPrefix.FindStringSubmatch(string(source))
	if prefix == nil || prefix[1] != extension.NamespacePrefix {
		t.Errorf("%s must declare EXTENSION_JOB_PREFIX = %q, the backend's extension.NamespacePrefix, or an extension agent reads as a bare agent", provenanceSource, extension.NamespacePrefix)
	}
}

// The census reads a dispatcher and nothing else: a workspace child is no
// principal, and a fragment with several actions keeps each one apart.
func TestTheExtensionCensusReadsDispatchersOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	unit := filepath.Join(root, "sample", "api")
	if err := os.MkdirAll(unit, 0o755); err != nil {
		t.Fatal(err)
	}
	fragment := "actions:\n" +
		"  - target: $.kinds['ext_sample_tick']\n    update:\n      role: dispatcher\n" +
		"  - target: $.kinds['ext_sample_tick_ws']\n    update:\n      role: workspace\n" +
		"  - target: $.kinds['ext_sample_sweep']\n    update:\n      job: sweep\n      role: dispatcher\n"
	if err := os.WriteFile(filepath.Join(unit, "jobs.yaml"), []byte(fragment), 0o600); err != nil {
		t.Fatal(err)
	}
	got := sortedKeys(extensionDispatcherKinds(t, root))
	want := []string{"ext_sample_sweep", "ext_sample_tick"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("census read %v, want %v", got, want)
	}
}

// jobsYAMLAction is one overlay action of a unit's api/jobs.yaml.
var jobsYAMLAction = regexp.MustCompile(`(?m)^\s*- target: \$\.kinds\['([a-z0-9_]+)'\]`)

// jobsYAMLDispatcher marks an action whose kind is the cadenced dispatcher.
var jobsYAMLDispatcher = regexp.MustCompile(`(?m)^\s*role: dispatcher\s*$`)

// extensionDispatcherKinds reads every dispatcher kind the units under root
// declare, valued by the file that declares it.
func extensionDispatcherKinds(t *testing.T, root string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*", "api", "jobs.yaml"))
	if err != nil {
		t.Fatalf("listing jobs fragments under %s: %v", root, err)
	}
	kinds := map[string]string{}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		text := string(source)
		starts := jobsYAMLAction.FindAllStringSubmatchIndex(text, -1)
		for i, at := range starts {
			end := len(text)
			if i+1 < len(starts) {
				end = starts[i+1][0]
			}
			if jobsYAMLDispatcher.MatchString(text[at[0]:end]) {
				kinds[text[at[2]:at[3]]] = file
			}
		}
	}
	return kinds
}

// extensionDeclaredJobs reads the `Jobs:` entries of every unit's Go code,
// keyed by the dispatcher kind extension.JobDeclaration derives for that unit
// and job. The unit is its directory's name.
func extensionDeclaredJobs(t *testing.T, root string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*", "*.go"))
	if err != nil {
		t.Fatalf("listing units under %s: %v", root, err)
	}
	declared := map[string]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := gatekit.ParseFile(file, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		unit := extension.Name(filepath.Base(filepath.Dir(file)))
		for _, job := range declaredJobNames(parsed) {
			kind := extension.JobDeclaration{Unit: unit, Job: job}.DispatcherKind()
			if kind == "" {
				t.Errorf("%s: unit %q is not a valid extension name, so job %q has no dispatcher kind", file, unit, job)
				continue
			}
			declared[kind] = file
		}
	}
	return declared
}

// declaredJobNames reads the Name of each element of a `Jobs:` field.
func declaredJobNames(file *ast.File) []string {
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.KeyValueExpr)
		if !ok || !namedKey(field.Key, "Jobs") {
			return true
		}
		list, ok := field.Value.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, element := range list.Elts {
			if name, ok := literalField(element, "Name"); ok {
				names = append(names, name)
			}
		}
		return false
	})
	return names
}

func namedKey(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// literalField reads one string field of a composite literal.
func literalField(expr ast.Expr, key string) (string, bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	for _, element := range lit.Elts {
		if field, ok := element.(*ast.KeyValueExpr); ok && namedKey(field.Key, key) {
			return gatekit.StringExpr(field.Value, nil, gatekit.FoldStrict)
		}
	}
	return "", false
}
