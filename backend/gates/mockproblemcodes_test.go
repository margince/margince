// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every Problem a frontend test, story or fixture answers carries a code the
// server can send. A mock with a code no handler emits never reaches the
// screen's branch or copy for it, so its test proves nothing about either.
//
// A product file that builds a Problem of its own marks it with
// `problem-code:client` and a reason, on the line or the one above.

import (
	"fmt"
	"go/parser"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// The floors sit well under what the census and the scan read. A walk gone
	// blind fails them, and a code or mock retired here and there does not.
	emittedCodeFloor = 150
	problemMockFloor = 300
	clientCodeMarker = "problem-code:client"
)

// problemTakerOwners are the frontend modules whose exported functions take a
// Problem as their first parameter, named `problem`.
var problemTakerOwners = []string{"frontend/src/lib/problem.ts", "frontend/src/lib/problembody.ts"}

// moduleFileOwner declares MODULE_FILE, the extensions a bundler resolves,
// which the frontend's own walks read too.
const moduleFileOwner = "frontend/scripts/lib/source-tree.ts"

var moduleFileDeclaration = regexp.MustCompile(`(?m)^export const MODULE_FILE = /(.+)/;$`)

// mockFile names a file that describes rather than ships: a test, story, kit,
// fixture or mock, or a file under e2e/ or testing/.
var mockFile = regexp.MustCompile(`(?i)(\.(test|spec|stories|testkit|fixtures?)\.|mock|(^|/)(e2e|testing)/)`)

func TestEveryProblemMockAnswersACodeTheServerSends(t *testing.T) {
	t.Parallel()
	emitted := emittedProblemCodes(t)
	if len(emitted) < emittedCodeFloor {
		t.Fatalf("read %d Problem codes from the server and expected at least %d: the census has gone blind to "+
			"the sites that set one", len(emitted), emittedCodeFloor)
	}
	for _, code := range contractProblemExampleCodes(t) {
		if _, read := emitted[code]; !read {
			t.Errorf("%s shows a Problem with code %q, and the census read no site that sends it: a site the "+
				"census cannot see, or an example of a code nothing sends", contractPath, code)
		}
	}
	mocks, scans := frontendProblemMocks(t)
	if len(mocks) < problemMockFloor {
		t.Fatalf("read %d Problem mocks in the frontend and expected at least %d: the scan has gone blind",
			len(mocks), problemMockFloor)
	}
	vouched, faults := clientMadeSites(scans)
	for _, fault := range faults {
		t.Error(fault)
	}
	known := map[string]bool{}
	for code := range emitted {
		known[code] = true
	}
	statusCodes := sharedStatusCodes(t)
	for _, mock := range mocks {
		if vouched[mock] {
			continue
		}
		if message, bad := judgeProblemMock(mock, known, statusCodes); bad {
			t.Error(message)
		}
	}
}

// judgeProblemMock refuses a code no server path sends, or one built at run
// time, and names the codes httperr's shared writers send with its status.
func judgeProblemMock(mock problemMock, known map[string]bool, statusCodes map[int][]string) (string, bool) {
	at := fmt.Sprintf("%s:%d", mock.path, mock.line)
	if mock.computed {
		return fmt.Sprintf("%s: a Problem mock builds its code at run time, so no check can read it; spell the code", at), true
	}
	if known[mock.code] {
		return "", false
	}
	hint := ""
	if sent := statusCodes[mock.status]; len(sent) > 0 {
		hint = fmt.Sprintf(" (for status %d the server's shared writers send %s)", mock.status, quoteAll(sent))
	}
	return fmt.Sprintf("%s: a Problem mock answers code %q, which no server path sends%s; answer the code the "+
		"handler this test stands in for sends", at, mock.code, hint), true
}

func quoteAll(texts []string) string {
	quoted := make([]string, len(texts))
	for i, text := range texts {
		quoted[i] = fmt.Sprintf("%q", text)
	}
	return strings.Join(quoted, ", ")
}

// clientMadeSites answers the mocks a `problem-code:client` marker vouches
// for. It counts in a `//` comment of a product file, with a reason. It vouches
// only for a literal the same file builds on its line or the next. An imported
// code is judged where it lands, so a product-named helper vouches for no test.
func clientMadeSites(scans map[string]*problemFileScan) (vouched map[problemMock]bool, faults []string) {
	vouched = map[problemMock]bool{}
	for filePath, scan := range scans {
		for _, marker := range scan.markers {
			line := scan.reader.lineOfPos(marker.pos)
			_, reason, _ := strings.Cut(marker.text, clientCodeMarker)
			at := fmt.Sprintf("%s:%d", filePath, line)
			switch {
			case !marker.line:
				continue
			case mockFile.MatchString(filePath):
				faults = append(faults, at+": "+clientCodeMarker+" in a test file; a mock answers a code the "+
					"screen receives, it cannot declare one")
				continue
			case strings.TrimSpace(reason) == "":
				faults = append(faults, at+": "+clientCodeMarker+" needs the reason the client builds this Problem")
				continue
			}
			found := false
			for _, mock := range scan.mocks {
				if mock.own && (mock.line == line || mock.line == line+1) {
					vouched[mock], found = true, true
				}
			}
			if !found {
				faults = append(faults, at+": "+clientCodeMarker+" vouches for no code this file builds into a "+
					"Problem on its line or the next")
			}
		}
	}
	return vouched, faults
}

// frontendProblemMocks reads every tracked frontend module, its extensions'
// included, and answers the Problem codes they answer.
func frontendProblemMocks(t *testing.T) ([]problemMock, map[string]*problemFileScan) {
	t.Helper()
	takers := problemTakers(t)
	moduleFile := frontendModuleFile(t)
	scans := map[string]*problemFileScan{}
	for _, file := range trackedFiles(t) {
		if file.symlink || !moduleFile.MatchString(file.path) || !frontendLayer(file.path) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(repoRoot, file.path))
		if err != nil {
			t.Fatalf("reading %s: %v", file.path, err)
		}
		source := string(raw)
		if !strings.Contains(source, "code") {
			continue
		}
		tree, err := parseTS(source, file.path)
		if err != nil {
			t.Errorf("%s: %v; this gate cannot read the file, so a Problem mock in it would go unjudged", file.path, err)
			continue
		}
		scans[file.path] = scanProblemFile(file.path, source, tree, takers)
	}
	return collectMocks(scans), scans
}

// collectMocks reads each file's calls of helpers other files export, then
// answers every file's mocks in path and line order.
func collectMocks(scans map[string]*problemFileScan) []problemMock {
	for _, scan := range scans {
		readImportedHelpers(scan, scans)
	}
	var mocks []problemMock
	for _, scan := range scans {
		mocks = append(mocks, scan.mocks...)
	}
	slices.SortFunc(mocks, func(a, b problemMock) int {
		return strings.Compare(fmt.Sprintf("%s:%09d:%s:%v", a.path, a.line, a.code, a.own),
			fmt.Sprintf("%s:%09d:%s:%v", b.path, b.line, b.code, b.own))
	})
	return slices.Compact(mocks)
}

// frontendModuleFile reads MODULE_FILE from the module that declares it.
func frontendModuleFile(t *testing.T) *regexp.Regexp {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, moduleFileOwner))
	if err != nil {
		t.Fatalf("reading %s: %v", moduleFileOwner, err)
	}
	declared := moduleFileDeclaration.FindSubmatch(raw)
	if declared == nil {
		t.Fatalf("%s declares no MODULE_FILE in the `export const MODULE_FILE = /…/;` shape this gate reads", moduleFileOwner)
	}
	pattern, err := regexp.Compile(string(declared[1]))
	if err != nil {
		t.Fatalf("%s: MODULE_FILE is no pattern Go can read: %v", moduleFileOwner, err)
	}
	return pattern
}

func frontendLayer(filePath string) bool {
	return strings.HasPrefix(filePath, "frontend/src/") || strings.HasPrefix(filePath, "frontend/e2e/") ||
		strings.HasPrefix(filePath, "extensions/") && slices.Contains(strings.Split(filePath, "/"), "frontend")
}

func parseTS(source, filePath string) (*tsTree, error) {
	tokens, comments, err := tokenizeTS(source, strings.HasSuffix(filePath, "x"))
	if err != nil {
		return nil, err
	}
	tree, err := buildTSTree(tokens)
	if err == nil {
		tree.comments = comments
	}
	return tree, err
}

// readImportedHelpers reads a file's calls of a helper another file exports,
// at the argument the helper makes a Problem's code.
func readImportedHelpers(scan *problemFileScan, scans map[string]*problemFileScan) {
	for _, call := range scan.calls {
		imported := scan.imports[scan.reader.tree.word(call)]
		for _, candidate := range []string{"", ".ts", ".tsx", ".js", ".jsx", "/index.ts", "/index.tsx"} {
			owner := scans[path.Clean(imported.from+candidate)]
			if owner == nil {
				continue
			}
			if source, isHelper := owner.helpers[imported.name]; isHelper && exports(owner, imported.name) {
				scan.reader.readCall(call, source)
			}
		}
	}
}

func exports(scan *problemFileScan, name string) bool {
	return slices.ContainsFunc(scan.funcs, func(fn tsFunc) bool { return fn.name == name && fn.exported })
}

// problemTakers reads the functions and classes the frontend's Problem module
// exports whose first parameter is named `problem`.
func problemTakers(t *testing.T) map[string]bool {
	t.Helper()
	takers := map[string]bool{}
	for _, owner := range problemTakerOwners {
		raw, err := os.ReadFile(filepath.Join(repoRoot, owner))
		if err != nil {
			t.Fatalf("reading %s: %v", owner, err)
		}
		tree, err := parseTS(string(raw), owner)
		if err != nil {
			t.Fatalf("%s: %v", owner, err)
		}
		for name := range problemTakersIn(tree) {
			takers[name] = true
		}
	}
	if !takers["ProblemError"] || len(takers) < 3 {
		t.Fatalf("read %v as the functions that take a Problem, and ProblemError is not among them: the read has "+
			"gone blind to %s", takers, strings.Join(problemTakerOwners, " and "))
	}
	return takers
}

func problemTakersIn(tree *tsTree) map[string]bool {
	takers := map[string]bool{}
	class := ""
	for i := range tree.tokens {
		switch {
		case tree.word(i) == "class" && tree.word(i-1) == "export":
			class = tree.word(i + 1)
		case tree.word(i) == "constructor" && tree.is(i+1, "(") && tree.word(i+2) == "problem" && class != "":
			takers[class] = true
		case tree.word(i) == "function" && tree.word(i-1) == "export" && tree.is(i+2, "(") && tree.word(i+3) == "problem":
			takers[tree.word(i+1)] = true
		}
	}
	return takers
}

// contractProblemExampleCodes reads the code of every problem+json example
// the contract shows.
func contractProblemExampleCodes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("reading %s: %v", contractPath, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decoding %s: %v", contractPath, err)
	}
	// A shared response no operation references promises nothing a client
	// can receive, so its example is no claim about the server.
	referenced := map[string]bool{}
	for _, ref := range responseRef.FindAllSubmatch(raw, -1) {
		referenced[string(ref[1])] = true
	}
	unreferenced := map[*yaml.Node]bool{}
	for _, response := range mappingPairs(mappingValue(mappingValue(doc.Content[0], "components"), "responses")) {
		if !referenced[response[0].Value] {
			unreferenced[response[1]] = true
		}
	}
	var codes []string
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		if unreferenced[node] {
			return
		}
		for i := 0; node.Kind == yaml.MappingNode && i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "application/problem+json" {
				codes = append(codes, exampleCodes(node.Content[i+1])...)
			}
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(&doc)
	if len(codes) == 0 {
		t.Fatalf("read no problem+json example codes from %s: the walk has gone blind", contractPath)
	}
	return codes
}

var responseRef = regexp.MustCompile(`#/components/responses/([A-Za-z0-9_]+)`)

// exampleCodes reads `code` from a media type's example and each of its examples' value.
func exampleCodes(media *yaml.Node) []string {
	var codes []string
	for _, example := range append([]*yaml.Node{mappingValue(media, "example")}, mappingValues(mappingValue(media, "examples"))...) {
		if code := mappingValue(mappingValue(example, "value"), "code"); code != nil && code.Kind == yaml.ScalarNode {
			codes = append(codes, code.Value)
		}
		if code := mappingValue(example, "code"); code != nil && code.Kind == yaml.ScalarNode {
			codes = append(codes, code.Value)
		}
	}
	return codes
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; node != nil && node.Kind == yaml.MappingNode && i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func mappingPairs(node *yaml.Node) [][2]*yaml.Node {
	var pairs [][2]*yaml.Node
	for i := 0; node != nil && node.Kind == yaml.MappingNode && i+1 < len(node.Content); i += 2 {
		pairs = append(pairs, [2]*yaml.Node{node.Content[i], node.Content[i+1]})
	}
	return pairs
}

func mappingValues(node *yaml.Node) []*yaml.Node {
	var values []*yaml.Node
	for i := 1; node != nil && node.Kind == yaml.MappingNode && i < len(node.Content); i += 2 {
		values = append(values, node.Content[i])
	}
	return values
}

// The scan is planted with every shape it claims to read and the ones it must
// pass over, each line saying what it must answer.
func TestTheMockScanReadsEveryPlantedShape(t *testing.T) {
	t.Parallel()
	scans, sources := plantedScans(t)
	read := map[string]map[int][]string{}
	for _, mock := range collectMocks(scans) {
		code := mock.code
		if mock.computed {
			code = "computed"
		}
		if read[mock.path] == nil {
			read[mock.path] = map[int][]string{}
		}
		read[mock.path][mock.line] = append(read[mock.path][mock.line], code)
	}
	wanted := 0
	for virtual, source := range sources {
		want := plantedWants(source)
		wanted += len(want)
		lines := slices.Collect(maps.Keys(want))
		for line := range read[virtual] {
			lines = append(lines, line)
		}
		for _, line := range slices.Compact(slices.Sorted(slices.Values(lines))) {
			got, wantText := strings.Join(slices.Sorted(slices.Values(read[virtual][line])), " "),
				strings.Join(slices.Sorted(slices.Values(want[line])), " ")
			if got != wantText {
				t.Errorf("%s:%d: read %q, want %q", virtual, line, got, wantText)
			}
		}
	}
	if wanted < 40 {
		t.Fatalf("read %d planted expectations; the corpus has lost its markers", wanted)
	}
}

// plantedScans reads the planted files under the paths they stand for.
func plantedScans(t *testing.T) (map[string]*problemFileScan, map[string]string) {
	t.Helper()
	scans, sources := map[string]*problemFileScan{}, map[string]string{}
	for virtual, planted := range map[string]string{
		"planted/problemmocks.test.tsx": "gates/testdata/problemmocks.tsx.txt",
		"planted/problemmockhelper.ts":  "gates/testdata/problemmockhelper.ts.txt",
		"planted/client.ts":             "gates/testdata/problemclient.ts.txt",
	} {
		raw, err := os.ReadFile(planted)
		if err != nil {
			t.Fatalf("reading %s: %v", planted, err)
		}
		tree, err := parseTS(string(raw), virtual)
		if err != nil {
			t.Fatalf("%s: %v", planted, err)
		}
		sources[virtual] = string(raw)
		scans[virtual] = scanProblemFile(virtual, string(raw), tree, problemTakers(t))
	}
	return scans, sources
}

var plantedWant = regexp.MustCompile(`// want: (.+)$`)

func plantedWants(source string) map[int][]string {
	want := map[int][]string{}
	for i, line := range strings.Split(source, "\n") {
		if found := plantedWant.FindStringSubmatch(line); found != nil {
			want[i+1] = strings.Fields(found[1])
		}
	}
	return want
}

// A finding names the site, the code and the shared writers' codes for its
// status. A code the server sends is no finding.
func TestAProblemMockFindingNamesTheSiteAndTheServersCodes(t *testing.T) {
	t.Parallel()
	statusCodes := map[int][]string{403: {"permission_denied", "scope_exceeds_grantor"}}
	known := map[string]bool{"permission_denied": true}
	message, bad := judgeProblemMock(problemMock{path: "f.test.tsx", line: 7, code: "forbidden", status: 403}, known, statusCodes)
	for _, part := range []string{"f.test.tsx:7", `"forbidden"`, `"permission_denied", "scope_exceeds_grantor"`} {
		if !bad || !strings.Contains(message, part) {
			t.Errorf("the finding %q does not name %s", message, part)
		}
	}
	if message, bad := judgeProblemMock(problemMock{path: "f.test.tsx", line: 8, code: "permission_denied"}, known, statusCodes); bad {
		t.Errorf("a code the server sends was refused: %s", message)
	}
	if _, bad := judgeProblemMock(problemMock{path: "f.test.tsx", line: 9, code: "permission_denied", computed: true}, known, statusCodes); !bad {
		t.Error("a code built at run time was let through")
	}
}

// A marker vouches for its own file's literal on its line or the next. A
// string, a block comment, a test file, a helper and an import vouch for none.
func TestAMarkerVouchesOnlyForTheCodeItsFileBuilds(t *testing.T) {
	t.Parallel()
	scans, _ := plantedScans(t)
	collectMocks(scans)
	vouched, faults := clientMadeSites(scans)
	var codes []string
	for mock := range vouched {
		codes = append(codes, mock.path+":"+mock.code)
	}
	if strings.Join(codes, " ") != "planted/client.ts:gateway_unavailable" {
		t.Errorf("vouched for %q, want only the gateway's own literal", codes)
	}
	slices.Sort(faults)
	want := []string{"planted/client.ts:14:", "planted/client.ts:7:", "planted/client.ts:9:", "planted/problemmocks.test.tsx:"}
	if len(faults) != len(want) {
		t.Fatalf("faulted %q, want %d faults", faults, len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(faults[i], prefix) {
			t.Errorf("fault %q, want one at %s", faults[i], prefix)
		}
	}
}

// The census is planted with every site shape it reads, beside the real
// httperr, and with the three shapes it must refuse.
func TestTheEmittedCodeCensusReadsEveryPlantedSite(t *testing.T) {
	t.Parallel()
	const planted, virtual = "gates/testdata/problemcodesites.go.txt", "internal/planted/sites.go"
	raw, err := os.ReadFile(planted)
	if err != nil {
		t.Fatalf("reading %s: %v", planted, err)
	}
	file, err := parser.ParseFile(gatekit.SourceFileSet(), virtual, raw, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", planted, err)
	}
	units := append(readGoUnits(t, problemWriterDir),
		&goUnit{path: virtual, dir: path.Dir(virtual), file: file, imports: localImports(file)})
	census := newEmittedCensus(t, units)
	census.readSources()
	for _, code := range []string{
		"planted_literal", "planted_const", "planted_const_converted", "planted_param",
		"planted_passed_on", "planted_method", "planted_field", "planted_typed_first", "planted_typed_second",
		"planted_fault_return", "planted_assigned", "planted_ranged", "planted_elided",
	} {
		if _, read := census.codes[code]; !read {
			t.Errorf("the census did not read the planted %q", code)
		}
	}
	var faulted []string
	for _, fault := range census.faults {
		if strings.Contains(fault, virtual) {
			faulted = append(faulted, fault)
		} else {
			t.Errorf("a fault outside the planted file: %s", fault)
		}
	}
	for _, refused := range []string{`fmt.Sprintf("planted_%d", n)`, "strings.ToLower(s)", "return valueHelper"} {
		at := fmt.Sprintf("%s:%d:", virtual, lineAt(string(raw), strings.Index(string(raw), refused)))
		if !slices.ContainsFunc(faulted, func(fault string) bool { return strings.HasPrefix(fault, at) }) {
			t.Errorf("the census did not refuse %s at %s; it faulted %q", refused, at, faulted)
		}
	}
	if len(faulted) != 3 {
		t.Errorf("faulted %d planted sites, want 3: %q", len(faulted), faulted)
	}
}
