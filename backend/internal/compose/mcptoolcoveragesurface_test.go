// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The surfaces the coverage page reports, and the census that keeps it from
// reporting one of them as all of them.
//
// THE DEFECT THIS EXISTS FOR. The page rendered `servedSurface`, which is the
// CORE catalog assembled with no unit composed, and called it "the tools the
// assistant is offered". The lane's assistant is offered something else: the
// listing is scope-filtered per caller, and a boot that composes a unit adds
// that unit's tools to the same registry. Both sets happened to hold 73 tools,
// so the totals agreed and the disagreement underneath them went unread —
// seven tools on the page the lane is never shown, and seven shown to the lane
// that the page has no row for.
//
// A page whose headline is a census must not be able to count a different
// population than the one it names. That is rule 8 in the file whose whole job
// is the census, and the equal count is exactly how it hid.
//
// WHY THE MANIFEST IS THE CORPUS. compose cannot import a unit — each lives in
// its own Go module and the DAG forbids the edge — so the composed registry is
// unreachable from this package's tests and `servedSurface`'s own comment says
// the arithmetic is an installation's. What IS reachable is what the composer
// published: manifest.generated.json carries one entry per governed capability,
// and an agent tool is the `agent_tool` kind. Deriving from the publication
// keeps this from becoming a hand-kept second list of the extension tier, which
// is the copy that goes short while still passing.
package compose

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// extensionsDir is the unit tier, relative to this package.
const extensionsDir = "../../../extensions"

// toolIDPrefix is how a manifest entry names an agent tool: the `id` is
// `tool/<name>` where a job's is `job/<name>`. The KIND is what decides an
// entry is a tool; the prefix only carries the name.
const (
	toolIDPrefix  = "tool/"
	agentToolKind = "agent_tool"
	manifestName  = "manifest.generated.json"
)

// unitTool is one extension tool as its unit published it.
type unitTool struct {
	Name string
	Unit string
}

// unitManifest is the slice of manifest.generated.json this census reads. The
// struct names only what it uses: a manifest carries six capability sections
// and gaining a seventh must not break a reader that wants one of them.
type unitManifest struct {
	Name      string `json:"name"`
	RiskTiers []struct {
		ID   string `json:"id"`
		Unit string `json:"unit"`
		Kind string `json:"kind"`
	} `json:"risk_tiers"`
}

// extensionTools reads every shipped unit's manifest and returns the agent
// tools the tier contributes to the registry, sorted.
//
// PRESENCE UNDER extensions/ IS THE ENABLEMENT (AGENTS.md), so the directory
// listing is the shipped set and no allowlist is kept here. A tree with no
// units returns nothing, which is the honest answer for a core-only build and
// not a scan that failed.
func extensionTools(dir string) ([]unitTool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the extension tier at %s: %w", dir, err)
	}
	var tools []unitTool
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), manifestName)
		body, readErr := os.ReadFile(path)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		var manifest unitManifest
		if unmarshalErr := json.Unmarshal(body, &manifest); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		for _, entry := range manifest.RiskTiers {
			if entry.Kind != agentToolKind {
				continue
			}
			unit := entry.Unit
			if unit == "" {
				unit = manifest.Name
			}
			tools = append(tools, unitTool{Name: strings.TrimPrefix(entry.ID, toolIDPrefix), Unit: unit})
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// lanePassportScopes are the scopes scripts/e2e-llm.sh mints its passport with.
//
// It is the LANE'S OWN CHOICE, restated here because the page's central claim is
// about the surface that passport is served, and a claim about a caller must
// name the caller. If the lane widens its passport, this list is what must move
// with it — TestTheLaneCanReachEveryToolACaseRequires fails until it does.
var lanePassportScopes = map[principal.Scope]bool{
	principal.ScopeRead:  true,
	principal.ScopeWrite: true,
}

// TestTheCoveragePageCountsTheSurfaceItNames is the census.
//
// THE CLAIM IT HAS TO EARN. This page's headline is a count of "tools the
// assistant is offered", and the defect it was built for is that the count named
// one population and measured another: the page rendered the all-scope core
// catalog while the lane's assistant is served a scope-filtered listing. Both
// held 73, so the totals agreed and the disagreement underneath went unread.
//
// So this asserts SET MEMBERSHIP, tool by tool, in both directions. A count here
// would restate the bug as the assertion.
func TestTheCoveragePageCountsTheSurfaceItNames(t *testing.T) {
	report := readPublishedCoverage(t)

	published := map[string]bool{}
	for _, row := range report.Tools {
		published[row.Name] = true
	}
	served := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		served[spec.Name] = true
	}

	for name := range served {
		if !published[name] {
			t.Errorf("the core catalog serves %s and the page has no row for it — the page counts "+
				"a smaller population than the one its headline names", name)
		}
	}
	for name := range published {
		if !served[name] {
			t.Errorf("the page carries a row for %s and the core catalog does not serve it — the "+
				"page counts a tool nothing offers", name)
		}
	}
}

// TestTheLaneCanReachEveryToolACaseRequires is the half the equal counts hid.
//
// A case that REQUIRES a tool its passport is never served cannot pass, ever,
// and it fails looking exactly like a model that chose not to call it. Seven
// tools on this page are outside the lane's scopes; a scenario naming one is a
// paid run bought to learn nothing.
//
// It reads the scenarios rather than the page's `driven` flag on purpose: the
// flag is derived from the same files, so checking it against them would compare
// a value with itself.
func TestTheLaneCanReachEveryToolACaseRequires(t *testing.T) {
	scope := map[string]principal.Scope{}
	for _, spec := range servedSurface(t).Specs() {
		scope[spec.Name] = spec.RequiredScope
	}

	cases, err := readE2ELLMCases(e2eLLMScenarioDir, e2eLLMRecordDir)
	if err != nil {
		t.Fatalf("reading the use-case lane: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("read no scenario from %s — a scan finding none proves nothing about the ones "+
			"that exist", e2eLLMScenarioDir)
	}

	required := 0
	for _, c := range cases {
		for _, tool := range c.Requires {
			required++
			want, known := scope[tool]
			if !known {
				t.Errorf("case %s requires %s and the served catalog has no such tool — the case "+
					"can never pass, and it fails looking like a model that chose not to call it",
					c.Name, tool)
				continue
			}
			if !lanePassportScopes[want] {
				t.Errorf("case %s requires %s, which needs scope %q — the lane's passport carries "+
					"only %v, so the tool is never in the listing the assistant is shown. Every run "+
					"of this case is paid for and lost.",
					c.Name, tool, want, sortedScopes())
			}
		}
	}
	// Under-recognition guard: a reader that resolved no requirement would pass
	// this test having checked nothing, which is the shape of failure this file
	// exists to refuse.
	if required == 0 {
		t.Fatal("no scenario declares a required tool — this census checked nothing and reported PASS")
	}
}

func sortedScopes() []string {
	out := make([]string, 0, len(lanePassportScopes))
	for s := range lanePassportScopes {
		out = append(out, string(s))
	}
	sort.Strings(out)
	return out
}

// readPublishedCoverage reads the committed page's JSON — the artefact a reader
// actually consults, not the value this package would compute a second time.
func readPublishedCoverage(t *testing.T) mcpToolCoverage {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(mcpDocsDir, "mcp-tool-coverage.json"))
	if err != nil {
		t.Fatalf("reading the published coverage page: %v", err)
	}
	var report mcpToolCoverage
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("decoding the published coverage page: %v", err)
	}
	if len(report.Tools) == 0 {
		t.Fatal("the published page carries no tool rows — every comparison below would agree with it vacuously")
	}
	return report
}

// TestTheUnitToolCensusReadsTheWholeTier keeps the manifest reader honest.
//
// It asserts the READER reached the tier, not that the tier ships tools: a tree
// whose units contribute none is a legitimate state (and is this tree's state),
// while a reader that found no manifest at all has failed short and would report
// that same emptiness.
func TestTheUnitToolCensusReadsTheWholeTier(t *testing.T) {
	manifests, err := unitManifestCount(extensionsDir)
	if err != nil {
		t.Fatalf("reading the unit manifests under %s: %v", extensionsDir, err)
	}
	if manifests == 0 {
		t.Fatalf("read no unit manifest under %s — the tier ships units, so a census finding no "+
			"manifest has failed short rather than measured a core-only build", extensionsDir)
	}

	units, err := extensionTools(extensionsDir)
	if err != nil {
		t.Fatalf("reading the unit tools: %v", err)
	}
	core := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		core[spec.Name] = true
	}
	for _, tool := range units {
		if core[tool.Name] {
			t.Errorf("unit %s publishes %s and the core catalog already serves that name — "+
				"a collision the boot rejects, and a page showing one row for it would report "+
				"two governed capabilities as one", tool.Unit, tool.Name)
		}
	}
}

// unitManifestCount is how many units published a manifest at all.
func unitManifestCount(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading the extension tier at %s: %w", dir, err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(dir, entry.Name(), manifestName)); statErr == nil {
			count++
		}
	}
	return count, nil
}

// corpusDir is the certification lane's scenario corpus, relative to this
// package. It is the OTHER lane that grades this surface, and the reason the
// coverage page's "no case requires this" is not the same sentence as "nothing
// has ever graded this".
const corpusDir = "aicert/corpus"

// corpusGradedTools returns, per tool name, the certification tasks whose
// scenarios name it — as the expected answer or as a near miss.
//
// A near miss COUNTS as graded, and that is the point rather than a looseness.
// The certification corpus grades tool SELECTION: a scenario naming a tool in
// `near_misses:` is asserting that a model must NOT reach for it on that goal,
// which is a live assertion about that tool's description. The use-case lane
// cannot express it at all — check.py matches a tool NAME appearing in a
// transcript, so it cannot tell a right first pick from a wrong one recovered.
//
// Read as TEXT rather than parsed as YAML. The corpus is the corpus of another
// lane and its schema is that lane's to change; a reader that only needs "is
// this word in this file" should not acquire an opinion about the file's shape,
// because the day the shape moves is the day this silently returns nothing.
// Matching a whole word is what keeps `read_record` out of `read_record_tags`.
// A MISSING CORPUS IS AN ERROR, not an empty one. The path is a fixed location
// in this repository, so its absence means the corpus moved and this reader did
// not — and answering "nothing is graded elsewhere" to that would publish the
// larger untried number with nothing failing. Under-recognition again, and the
// direction rule 8 names.
func corpusGradedTools(dir string, names []string) (map[string][]string, error) {
	tasks, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the certification corpus at %s: %w", dir, err)
	}
	graded := map[string][]string{}
	for _, task := range tasks {
		if !task.IsDir() {
			continue
		}
		scenarios, readErr := os.ReadDir(filepath.Join(dir, task.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var body strings.Builder
		for _, scenario := range scenarios {
			if !strings.HasSuffix(scenario.Name(), ".yaml") {
				continue
			}
			text, fileErr := os.ReadFile(filepath.Join(dir, task.Name(), scenario.Name()))
			if fileErr != nil {
				return nil, fileErr
			}
			body.Write(text)
			body.WriteString("\n")
		}
		corpus := body.String()
		for _, name := range names {
			if namedInCorpus(corpus, name) {
				graded[name] = append(graded[name], task.Name())
			}
		}
	}
	for name := range graded {
		sort.Strings(graded[name])
	}
	return graded, nil
}

// namedInCorpus reports whether a tool name appears as a whole word.
//
// The boundary is checked against the characters a tool name is built from
// rather than with a regex per tool: the corpus is read once per task and the
// name set is the whole catalog, so this runs tens of thousands of times.
func namedInCorpus(corpus, name string) bool {
	for from := 0; ; {
		at := strings.Index(corpus[from:], name)
		if at < 0 {
			return false
		}
		at += from
		before := at == 0 || !isToolNameByte(corpus[at-1])
		end := at + len(name)
		after := end == len(corpus) || !isToolNameByte(corpus[end])
		if before && after {
			return true
		}
		from = at + 1
	}
}

func isToolNameByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// writeCoverageSurfaces is the section this page did not have, and the reason
// its headline number was unreadable.
//
// There are TWO surfaces and they are not comparable. Surface A is the MCP
// catalog a person's assistant is offered — broad, because a person is there to
// correct a wrong reach. Surface B is a scheduled agent's declared allowlist —
// five to seven tools, because the run is unattended and every listed tool is
// paid for on every step of every run. Reporting one coverage number over both
// reports a menu nobody is served.
//
// It also states what this page CANNOT see, rather than leaving the reader to
// infer that silence means zero: the units' tools, and the scope filter.
func writeCoverageSurfaces(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## The two surfaces\n\n")
	p.WriteString("A tool being \"untried\" means something different on each, so the numbers above " +
		"are Surface A's.\n\n")
	p.WriteString("| | Surface A — MCP | Surface B — scheduled agents |\n|---|---|---|\n")
	p.WriteString("| Who drives it | a person, watching | a job on a timer, unattended |\n")
	fmt.Fprintf(p, "| Menu | %d tools, the whole catalog | %d tools, declared per agent |\n",
		r.Totals.Tools, smallestAgentMenu(r.Agents))
	p.WriteString("| A wrong reach | the person corrects it | nobody is there |\n")
	p.WriteString("| Graded by | the use-case lane on this page | " +
		"[ai-certification.md](ai-certification.md) |\n\n")

	p.WriteString("### Surface B, as the contract declares it\n\n")
	p.WriteString("From `backend/api/ai-tasks.yaml`. The allowlist NARROWS and never grants — every " +
		"call still passes the same admission gate\n")
	p.WriteString("against the same passport — and an empty one is a build error, because it would " +
		"read as \"no narrowing\" and hand back the whole catalog.\n\n")
	p.WriteString("| Agent | Tools | Attaches |\n|---|---:|---|\n")
	for _, a := range r.Agents {
		fmt.Fprintf(p, "| `%s` | %d | %s |\n", a.Name, len(a.Tools), joinOrDash(a.Tools))
	}
	p.WriteString("\n")

	p.WriteString("### What this page cannot see\n\n")
	fmt.Fprintf(p, "**The shipped units add %d more tools to the same registry**, and this page "+
		"cannot price them.\n", r.Totals.UnitTools)
	p.WriteString("A unit is its own Go module and the architecture forbids the core importing one, " +
		"so the composed catalog is unreachable\n")
	p.WriteString("from the package that generates this page. The names below come from what each " +
		"unit published; the token cost is an\n")
	p.WriteString("installation's own arithmetic. No use case requires any of them.\n\n")
	p.WriteString("| Tool | Unit |\n|---|---|\n")
	for _, tool := range r.UnitTools {
		fmt.Fprintf(p, "| `%s` | `%s` |\n", tool.Name, tool.Unit)
	}
	if len(r.UnitTools) == 0 {
		p.WriteString("| _no unit ships a tool_ | — |\n")
	}
	p.WriteString("\n")
	p.WriteString("**The listing is also scope-filtered per caller.** A tool on this page is offered " +
		"to a caller whose passport carries its\n")
	p.WriteString("scope, and to no other — so a reader must not read a row here as \"every " +
		"assistant sees this\". A case cannot drive a tool\n")
	p.WriteString("its passport is not served, and one that tried would fail for a reason that is " +
		"not the product's.\n\n")
}

// smallestAgentMenu is the narrowest declared allowlist, for the surface
// comparison. Zero when no agent is declared, which the caller renders as-is
// rather than hiding.
func smallestAgentMenu(agents []agentSurfaceRow) int {
	smallest := 0
	for _, a := range agents {
		if smallest == 0 || len(a.Tools) < smallest {
			smallest = len(a.Tools)
		}
	}
	return smallest
}

// TestTheScenarioReaderSeesACommentedList holds the fix for a reader that failed
// short in exactly the way this page's headline did.
//
// The scenarios carry their reasoning inline, a comment above the tool it
// explains. The block patterns admitted only item lines, so a `may_call:` whose
// FIRST line is a comment matched NOTHING — not a truncated list, an empty one.
// Eight cases published no permitted tools while permitting three to five each,
// and the same helper feeds the write declaration, so a writing tool listed
// after a comment was invisible to the gate that decides whether a case gets a
// fresh database.
//
// The fixture is inline rather than a scenario file: the defect is in the
// READER, and a corpus that happens to contain the shape today is a corpus that
// can stop containing it (AGENTS.md rule 8 — plant the case).
func TestTheScenarioReaderSeesACommentedList(t *testing.T) {
	const scenario = `name: planted
must_call:
  # the comment that broke this
  - alpha
  - beta

may_call:
  # a leading comment, which is the shape that matched nothing
  - gamma
  # an interleaved one
  - delta
`
	must := toolsInBlock(e2eMustCallBlock, e2eMustCallInline, scenario)
	may := toolsInBlock(e2eMayCallBlock, e2eMayCallInline, scenario)

	if want := []string{"alpha", "beta"}; !equalStrings(must, want) {
		t.Errorf("must_call read as %v, want %v — a comment above an item hides the items after it", must, want)
	}
	if want := []string{"gamma", "delta"}; !equalStrings(may, want) {
		t.Errorf("may_call read as %v, want %v — a block opening with a comment must not read as empty", may, want)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
