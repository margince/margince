// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The two source censuses that used to be awk, and the corpus that holds both
// halves of each of them to the same cases.
//
// The rule is singular and the PARSER is not: Go is read by go/ast here and
// TypeScript by ts.createSourceFile in frontend/src/quality/moneyscale.test.ts.
// That split is not a second implementation of the rule — it is the ABSENCE of
// a hand-written lexer, and the split already existed when the scanner was awk,
// spelled by hand and undeclared in a backslash rule, a `${…}` handler and a
// FILENAME test. What must stay singular is the rule and the cases, and
// testdata/sourcecensus.json is where they are.
//
// Every case in it was planted because something got through.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// censusCase is one planted case: source that must be refused, or must not be.
type censusCase struct {
	Gate string `json:"gate"`
	Lang string `json:"lang"`
	// Arm names the behaviour the case is about, and it is what the drift gate
	// below keys on: a rule arm exercised in one language and not the other is
	// how two halves of one rule start to disagree.
	Arm  string `json:"arm"`
	Want string `json:"want"`
	Name string `json:"name"`
	Body string `json:"body"`
	// Must and MustNot are the one-spelling suite's own assertion that a
	// finding names the right line: a waiver that silenced the file would still
	// report SOMETHING, and only naming the token tells the two apart.
	Must    string `json:"must,omitempty"`
	MustNot string `json:"must_not,omitempty"`
}

const censusCorpus = "gates/testdata/sourcecensus.json"

func readCensusCorpus(t *testing.T) []censusCase {
	t.Helper()
	raw, err := os.ReadFile(censusCorpus)
	if err != nil {
		t.Fatalf("reading the planted cases: %v", err)
	}
	var cases []censusCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding %s: %v", censusCorpus, err)
	}
	if len(cases) == 0 {
		t.Fatal("the corpus is empty, so every case below would pass having tested nothing")
	}
	return cases
}

// TestTheMoneyScaleCensusAnswersEveryPlantedGoCase runs the rule over the cases
// that were written for it, one temporary file at a time.
func TestTheMoneyScaleCensusAnswersEveryPlantedGoCase(t *testing.T) {
	t.Parallel()
	ran := 0
	for _, planted := range readCensusCorpus(t) {
		if planted.Gate != "money-scale" || planted.Lang != "go" {
			continue
		}
		ran++
		t.Run(planted.Name, func(t *testing.T) {
			path := plantGo(t, planted.Body)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found, err := gatekit.MoneyScaleFindings(path, src)
			assertPlanted(t, planted, found, err)
		})
	}
	if ran == 0 {
		t.Fatal("no Go money-scale cases ran, so this suite proves nothing")
	}
}

func TestTheOneSpellingCensusAnswersEveryPlantedCase(t *testing.T) {
	t.Parallel()
	codes := sqlstateCodes(t)
	ran := 0
	for _, planted := range readCensusCorpus(t) {
		if planted.Gate != "one-spelling" {
			continue
		}
		ran++
		t.Run(planted.Name, func(t *testing.T) {
			path := plantGo(t, planted.Body)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found, err := gatekit.OneSpellingFindings(path, src, codes)
			assertPlanted(t, planted, found, err)
		})
	}
	if ran == 0 {
		t.Fatal("no one-spelling cases ran, so this suite proves nothing")
	}
}

// plantGo writes one case's body as a compilable file. The header is the
// probe's, never the case's: a case is about ONE construct and should not have
// to carry a package clause to say so.
func plantGo(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.go")
	source := "// SPDX-License-Identifier: BUSL-1.1\npackage probe\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertPlanted scores one case. `unclosed` is its own expectation and NOT a
// flavour of `fires`, because the two mean opposite things about the run: fires
// says the census READ the code and found the defect, unclosed says it could
// not read the code at all and refused rather than pretend. Scoring one as the
// other would let a scanner that has stopped working satisfy every detection
// case in the suite.
func assertPlanted(t *testing.T, planted censusCase, found []gatekit.Finding, err error) {
	t.Helper()
	switch planted.Want {
	case "unclosed":
		if err == nil {
			t.Fatalf("the census read a file it cannot have finished, and reported %d finding(s)", len(found))
		}
	case "fires":
		if err != nil {
			t.Fatalf("the census refused to read the case instead of judging it: %v", err)
		}
		if len(found) == 0 {
			t.Fatal("the planted defect was not found")
		}
		assertNames(t, planted, found)
	case "silent":
		if err != nil {
			t.Fatalf("the census refused to read the case: %v", err)
		}
		if len(found) != 0 {
			t.Fatalf("the census refused code that is not the defect: %v", found)
		}
	default:
		t.Fatalf("unknown expectation %q — a case nothing scores is a case nothing tests", planted.Want)
	}
}

// assertNames holds a finding to the token it is about. A waiver that quieted
// the whole FILE would still report the other defect, and only the token tells
// that apart from a waiver that silenced its own line.
func assertNames(t *testing.T, planted censusCase, found []gatekit.Finding) {
	t.Helper()
	reported := make([]string, 0, len(found))
	for _, one := range found {
		reported = append(reported, one.String())
	}
	all := strings.Join(reported, "\n")
	if planted.Must != "" && !strings.Contains(all, planted.Must) {
		t.Errorf("the finding does not name %q, so it is not about the planted defect:\n%s", planted.Must, all)
	}
	if planted.MustNot != "" && strings.Contains(all, planted.MustNot) {
		t.Errorf("the finding names %q, which is waived on its own line:\n%s", planted.MustNot, all)
	}
}

// sqlstateCodes reads the codes from the file that OWNS them rather than
// retyping them here. A list typed in a gate is itself a second spelling of the
// census, and the sixth code somebody adds to storekit would be silently
// un-gated by the gate that exists to stop exactly that.
func sqlstateCodes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, sqlstateOwner))
	if err != nil {
		t.Fatalf("reading the SQLSTATE owner: %v", err)
	}
	codes := regexp.MustCompile(`"[0-9]{2}[0-9A-Z]{3}"`).FindAllString(string(raw), -1)
	if len(codes) == 0 {
		t.Fatalf("no SQLSTATE constants in %s — this gate is reading the wrong file", sqlstateOwner)
	}
	unique := map[string]bool{}
	for _, code := range codes {
		unique[strings.Trim(code, `"`)] = true
	}
	out := make([]string, 0, len(unique))
	for code := range unique {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

const sqlstateOwner = "backend/internal/platform/database/storekit/sqlstate.go"

// censusRoots are the trees both censuses read. A root that has moved makes the
// walk match nothing, and a census over an empty universe reports OK — the
// under-recognition failure a census must not have — so a missing root is a
// refusal here rather than a silent pass.
var censusRoots = []string{
	"backend/internal", "backend/cmd", "backend/pkg", "backend/tools", "extensions", "fixtures",
}

// censusFloor is the number of files below which the walk has plainly stopped
// reaching the tree. It is not a target: it is the tripwire for the day a
// changed root, a moved directory or a broken filter turns this into a census
// of nothing that still says OK.
const censusFloor = 400

// censusSourceFiles walks to the hand-written Go files the censuses judge. Tests are out
// of scope because they construct these errors and describe these defects on
// purpose, and generated files are not anybody's to fix.
func censusSourceFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range censusRoots {
		at := filepath.Join(repoRoot, root)
		if info, err := os.Stat(at); err != nil || !info.IsDir() {
			t.Fatalf("scan root %s does not exist — this census would inspect nothing and say OK", root)
		}
		if err := filepath.WalkDir(at, func(path string, entry os.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir():
				if entry.Name() == "node_modules" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			case !strings.HasSuffix(path, ".go"):
				return nil
			case strings.HasSuffix(path, "_test.go"),
				strings.HasSuffix(path, "_gen.go"), strings.HasSuffix(path, ".gen.go"):
				return nil
			}
			files = append(files, path)
			return nil
		}); err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if len(files) < censusFloor {
		t.Fatalf("the walk reached %d Go files, fewer than the %d this tree has — it has stopped "+
			"reaching the tree, and a census over what is left would report OK having read almost none of it",
			len(files), censusFloor)
	}
	return files
}

// TestNoMinorUnitAmountIsScaledByAHardCodedPowerOfTen is the money-scale census
// over the Go half of the tree.
//
// values/ is excluded because it OWNS the conversion: MajorUnits and MinorUnits
// are where the ISO-4217 digit table is applied, and a census refusing the
// implementation it points people at would be refusing the answer.
func TestNoMinorUnitAmountIsScaledByAHardCodedPowerOfTen(t *testing.T) {
	t.Parallel()
	var findings []string
	for _, path := range censusSourceFiles(t) {
		if strings.Contains(path, "shared/kernel/values/") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		found, err := gatekit.MoneyScaleFindings(path, src)
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, one := range found {
			findings = append(findings, one.String())
		}
	}
	if len(findings) > 0 {
		t.Errorf("an amount in minor units is scaled by a hard-coded power of ten. A currency with "+
			"no minor unit (VND, JPY, KRW) is then understated a hundredfold, and a three-decimal "+
			"one (KWD) is overstated tenfold — use values.MajorUnits / WholeMajorUnits / MinorUnits:\n  %s",
			strings.Join(findings, "\n  "))
	}
}

// TestSQLSTATEsRefusalsAndCurrencyShapesHaveOneSpelling is the one-spelling
// census over the tree.
//
// The owners are excluded, because a census that refused its own subject would
// be refusing the answer: sqlstate.go is where the codes are named, values/ is
// where the ISO-4217 shape is, and sourcecensus.go is where this census spells
// what it looks for. Excluding the FILES rather than the arms is the same thing
// here — no owner carries another's shape — and a reader can check it at a
// glance.
func TestSQLSTATEsRefusalsAndCurrencyShapesHaveOneSpelling(t *testing.T) {
	t.Parallel()
	codes := sqlstateCodes(t)
	var findings []string
	for _, path := range censusSourceFiles(t) {
		if isOneSpellingOwner(path) {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		found, err := gatekit.OneSpellingFindings(path, src, codes)
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, one := range found {
			findings = append(findings, one.String())
		}
	}
	if len(findings) > 0 {
		t.Errorf("a predicate this tree owns in one place is spelled again:\n  %s", strings.Join(findings, "\n  "))
	}
}

// TestEveryMoneyScaleArmIsPlantedInBothLanguages is the drift gate over the
// corpus, and it is the reason one corpus is enough for two parsers.
//
// The rule is singular; the parsers are not. What keeps them from disagreeing
// is that neither may exercise an arm the other does not — a `fires` case that
// exists only for Go says nothing about whether the TypeScript half still
// catches it, and the half nobody planted a case for is the half that quietly
// stops working.
func TestEveryMoneyScaleArmIsPlantedInBothLanguages(t *testing.T) {
	t.Parallel()
	languages := map[string]map[string]bool{}
	for _, planted := range readCensusCorpus(t) {
		if planted.Gate != "money-scale" {
			continue
		}
		if languages[planted.Arm] == nil {
			languages[planted.Arm] = map[string]bool{}
		}
		languages[planted.Arm][planted.Lang] = true
	}
	if len(languages) == 0 {
		t.Fatal("the corpus names no money-scale arms, so this gate compares nothing")
	}
	arms := make([]string, 0, len(languages))
	for arm := range languages {
		arms = append(arms, arm)
	}
	sort.Strings(arms)
	for _, arm := range arms {
		for _, lang := range []string{"go", "ts"} {
			if !languages[arm][lang] {
				t.Errorf("the %q arm has no %s case. The rule is one rule and the parsers are two, "+
					"so an arm planted in one language and not the other is the half that stops "+
					"working without anything failing", arm, lang)
			}
		}
	}
}

// isOneSpellingOwner reports whether a file is where one of the three
// predicates is DECLARED. See the census's own doc for why each one is here.
func isOneSpellingOwner(path string) bool {
	return strings.HasSuffix(path, "storekit/sqlstate.go") ||
		strings.Contains(path, "shared/kernel/values/") ||
		strings.HasSuffix(path, "gatekit/sourcecensus.go")
}
