// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var recut = flag.Bool("recut", false, "drop the stylebaseline.txt lines whose sentence passes its rule or is gone")

// wantStyleWaivers and wantStyleDigest pin the lines of stylebaseline.txt, so
// a line added, dropped or swapped by hand moves a value review sees.
const wantStyleWaivers = 157

const wantStyleDigest = "1725483ded77062584462990b8eca6fe029ea145cb31b304d44510d127bfd313"

var (
	styleWaiverPin = regexp.MustCompile(`(?m)^const wantStyleWaivers = \d+$`)
	styleDigestPin = regexp.MustCompile(`(?m)^const wantStyleDigest = "[0-9a-f]{64}"$`)
)

func TestTheStyleBaselineHoldsItsPinnedLines(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	if err := pinsHold(waivers, wantStyleWaivers, wantStyleDigest); err != nil {
		t.Error(err)
	}
}

// syntheticWaivers stand in for the baseline, which may shrink to no line.
var syntheticWaivers = []Waiver{
	{Rule: ruleEnDash, Text: "Saved — for now."},
	{Rule: ruleEnWe, Text: "We saved it."},
	{Rule: ruleViToi, Text: "Giao cho tôi", English: "Assign"},
}

func TestASwappedStyleWaiverBreaksTheDigestPin(t *testing.T) {
	t.Parallel()
	count, digest := len(syntheticWaivers), styleDigest(syntheticWaivers)
	if err := pinsHold(syntheticWaivers, count, digest); err != nil {
		t.Fatal(err)
	}
	swapped := slices.Clone(syntheticWaivers)
	swapped[0] = Waiver{Rule: ruleEnDash, Text: "A sentence no rewrite has reached — yet."}
	if pinsHold(swapped, count, digest) == nil {
		t.Error("a baseline with one line swapped for another still matches both pins")
	}
}

func pinsHold(waivers []Waiver, wantCount int, wantDigest string) error {
	if digest := styleDigest(waivers); len(waivers) != wantCount || digest != wantDigest {
		return fmt.Errorf("%s holds %d waivers with digest %s, and the pins say %d and %s. "+
			"The recut rewrites both pins when it drops lines, and a line dropped by hand sets both in the same change. "+
			"A line added or swapped fails make style-baseline-shrinks: rewrite the sentence instead",
			baselinePath, len(waivers), digest, wantCount, wantDigest)
	}
	return nil
}

func styleDigest(waivers []Waiver) string {
	lines := make([]string, len(waivers))
	for i, w := range waivers {
		lines[i] = w.line() + "\n"
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return hex.EncodeToString(sum[:])
}

func TestEveryStyleWaiverStillBreaksItsRule(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	passing := waiversThatPass(t, waivers)
	for i, w := range waivers {
		switch _, known := ruleNamed(w.Rule); {
		case !known:
			t.Errorf("%s waives %q under %s, which is no rule", baselinePath, w.Text, w.Rule)
		case passing[w]:
			t.Errorf("%s waives %q under %s, and it now passes: delete the line", baselinePath, w.Text, w.Rule)
		}
		if i > 0 && waiverOrder(waivers[i-1], w) >= 0 {
			t.Errorf("%s is not sorted and unique at %s; move the line", baselinePath, w.line())
		}
	}
}

// waiversThatPass answers the waivers whose sentence no longer breaks their
// known rule.
func waiversThatPass(t *testing.T, waivers []Waiver) map[Waiver]bool {
	t.Helper()
	src, err := loadSources()
	if err != nil {
		t.Fatal(err)
	}
	passing := map[Waiver]bool{}
	for _, w := range waivers {
		if rule, known := ruleNamed(w.Rule); known && !rule.breaks(src, w.Text, w.English) {
			passing[w] = true
		}
	}
	return passing
}

// TestEveryStyleWaiverNamesASentenceInTheTree fails a waiver whose sentence no
// Go string in the module spells any more, so a rewrite takes its line along.
// A sentence joined from a named constant is not folded and would read as gone.
func TestEveryStyleWaiverNamesASentenceInTheTree(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	spelled := sentencesInTheTree(t, waivers)
	for _, w := range waivers {
		if !spelled[w.Text] {
			t.Errorf("%s waives %q under %s, and no Go string spells it any more: delete the line",
				baselinePath, w.Text, w.Rule)
		}
	}
}

// sentencesInTheTree answers which waived sentences a Go string of the module
// spells.
func sentencesInTheTree(t *testing.T, waivers []Waiver) map[string]bool {
	t.Helper()
	wanted := map[string]bool{}
	for _, w := range waivers {
		wanted[w.Text] = false
	}
	files, root := 0, gatekit.ModuleRoot(t)
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !sourceFile(root, path) {
			return walkErr
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		ast.Inspect(file, func(node ast.Node) bool {
			switch node.(type) {
			case *ast.BasicLit, *ast.BinaryExpr:
				if text, ok := gatekit.StringExpr(node, nil, gatekit.FoldStrict); ok {
					if _, want := wanted[text]; want {
						wanted[text] = true
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := plainSourceCount(t, root); files != want {
		t.Fatalf("read %d Go files, and a plain walk of the module finds %d; the walk is reading another tree", files, want)
	}
	return wanted
}

// sourceFile is a hand-written, non-test Go file of the module.
func sourceFile(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if segment == "testdata" || (segment != "." && strings.HasPrefix(segment, ".")) {
			return false
		}
	}
	return true
}

func plainSourceCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil && !entry.IsDir() && sourceFile(root, path) {
			count++
		}
		return walkErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func waiverOrder(a, b Waiver) int {
	return cmp.Or(strings.Compare(a.Rule, b.Rule), strings.Compare(a.Text, b.Text), strings.Compare(a.English, b.English))
}

// TestRecutStyleBaseline drops the lines the two tests above fail for a
// sentence that passes or is gone. It never adds a line.
func TestRecutStyleBaseline(t *testing.T) {
	if !*recut {
		t.Skip("drops stale lines from stylebaseline.txt; run with -recut")
	}
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	passing, spelled := waiversThatPass(t, waivers), sentencesInTheTree(t, waivers)
	var kept []Waiver
	for _, w := range waivers {
		if !passing[w] && spelled[w.Text] {
			kept = append(kept, w)
		}
	}
	writeBaseline(t, "stylebaseline.txt", kept)
	writePins(t, "stylebaseline_test.go", kept)
	t.Logf("kept %d of %d waivers", len(kept), len(waivers))
}

func TestTheRecutRewritesBothPinsToTheLinesItKept(t *testing.T) {
	t.Parallel()
	kept, dir := syntheticWaivers, t.TempDir()
	pins, baseline := filepath.Join(dir, "pin_test.go"), filepath.Join(dir, "stylebaseline.txt")
	stale := "package x\n\nconst wantStyleWaivers = 160\n\nconst wantStyleDigest = \"" + strings.Repeat("0", 64) + "\"\n"
	if err := os.WriteFile(pins, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	writeBaseline(t, baseline, kept)
	writePins(t, pins, kept)
	written, err := os.ReadFile(baseline)
	if err != nil {
		t.Fatal(err)
	}
	reread, err := parseBaseline(string(written))
	if err != nil {
		t.Fatal(err)
	}
	want := "package x\n\nconst wantStyleWaivers = 3\n\nconst wantStyleDigest = \"" + styleDigest(reread) + "\"\n"
	if got, err := os.ReadFile(pins); err != nil || string(got) != want || len(reread) != len(kept) {
		t.Errorf("the recut wrote %d lines and left the pins as %q, want %q: %v", len(reread), got, want, err)
	}
}

func writePins(t *testing.T, path string, kept []Waiver) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range []*regexp.Regexp{styleWaiverPin, styleDigestPin} {
		if len(pin.FindAll(source, -1)) != 1 {
			t.Fatalf("%s no longer declares %s once on a line of its own", path, pin)
		}
	}
	source = styleWaiverPin.ReplaceAllLiteral(source, []byte("const wantStyleWaivers = "+strconv.Itoa(len(kept))))
	source = styleDigestPin.ReplaceAllLiteral(source, []byte(`const wantStyleDigest = "`+styleDigest(kept)+`"`))
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBaseline(t *testing.T, path string, waivers []Waiver) {
	t.Helper()
	var out bytes.Buffer
	for line := range strings.SplitSeq(baselineText, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		out.WriteString(line + "\n")
	}
	for _, w := range waivers {
		out.WriteString(w.line() + "\n")
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
