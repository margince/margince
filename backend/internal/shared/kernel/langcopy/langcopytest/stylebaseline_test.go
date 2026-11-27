// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
const wantStyleWaivers = 156

const wantStyleDigest = "c1965916fb8dd1866cd248ce5b3d0fb100833b3d13080ccf8c209c8be10bfef7"

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

// recutCommand runs the recut alone, from backend/: the tests that fail a stale
// line must not run beside the one that drops it.
const recutCommand = "go test ./internal/shared/kernel/langcopy/langcopytest -run '^TestRecutStyleBaseline$' -recut"

// TestRecutStyleBaseline drops the lines the two tests above fail for a
// sentence that passes or is gone. It never adds a line.
func TestRecutStyleBaseline(t *testing.T) {
	if !*recut {
		t.Skip("TestRecutStyleBaseline drops stale lines from stylebaseline.txt; run it from backend/ with: " + recutCommand)
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
	if err := recutFiles(os.Rename, "stylebaseline.txt", "stylebaseline_test.go", kept); err != nil {
		t.Fatal(err)
	}
	t.Logf("kept %d of %d waivers", len(kept), len(waivers))
}

func TestTheRecutCommandReadsTheSameWhereverItIsShown(t *testing.T) {
	t.Parallel()
	page := filepath.Join(gatekit.ModuleRoot(t), "..", "docs", "reference", "ui-copy-style.md")
	text, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	for shown, where := range map[string]string{string(text): page, baselineText: baselinePath} {
		if !strings.Contains(shown, recutCommand) {
			t.Errorf("%s does not show the recut command %s", where, recutCommand)
		}
	}
}

var stalePins = "package x\n\nconst wantStyleWaivers = 160\n\nconst wantStyleDigest = \"" + strings.Repeat("0", 64) + "\"\n"

func TestTheRecutRewritesBothPinsToTheLinesItKept(t *testing.T) {
	t.Parallel()
	kept, dir := syntheticWaivers, t.TempDir()
	pins, baseline := filepath.Join(dir, "pin_test.go"), filepath.Join(dir, "stylebaseline.txt")
	for path, text := range map[string]string{pins: stalePins, baseline: ""} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := recutFiles(os.Rename, baseline, pins, kept); err != nil {
		t.Fatal(err)
	}
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

func TestARecutThatCannotFinishLeavesBothFilesAsTheyWere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, pinned    string
		pinsRenameFails bool
	}{
		{"pins it cannot rewrite", "package x\n", false},
		{"a pins rename that fails", stalePins, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			pins, baseline := filepath.Join(dir, "pin_test.go"), filepath.Join(dir, "stylebaseline.txt")
			before := map[string]string{pins: tc.pinned, baseline: "en.we\t\"We saved it.\"\n"}
			for path, text := range before {
				if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			rename := func(from, to string) error {
				if tc.pinsRenameFails && to == pins {
					return errors.New("the disk is full")
				}
				return os.Rename(from, to)
			}
			if recutFiles(rename, baseline, pins, syntheticWaivers) == nil {
				t.Fatal("a recut that could not finish reported no error")
			}
			for path, want := range before {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Errorf("the failed recut left %s as %q, want it untouched as %q: %v", path, got, want, err)
				}
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != len(before) {
				t.Errorf("the failed recut left %d files behind in its directory, want %d: %v", len(entries), len(before), err)
			}
		})
	}
}

// recutFiles prepares the baseline and its pins, writes each beside its file
// and renames both into place. A pin it cannot rewrite leaves both files as
// they were, and so does a pins rename that fails: the old baseline goes back.
func recutFiles(rename func(from, to string) error, baselineFile, pinsFile string, kept []Waiver) error {
	source, err := os.ReadFile(pinsFile)
	if err != nil {
		return err
	}
	for _, pin := range []*regexp.Regexp{styleWaiverPin, styleDigestPin} {
		if len(pin.FindAll(source, -1)) != 1 {
			return fmt.Errorf("%s no longer declares %s once on a line of its own", pinsFile, pin)
		}
	}
	source = styleWaiverPin.ReplaceAllLiteral(source, []byte("const wantStyleWaivers = "+strconv.Itoa(len(kept))))
	source = styleDigestPin.ReplaceAllLiteral(source, []byte(`const wantStyleDigest = "`+styleDigest(kept)+`"`))
	original, err := os.ReadFile(baselineFile)
	if err != nil {
		return err
	}
	baselineStaged, err := stageBeside(baselineFile, baselineHolding(kept))
	if err != nil {
		return err
	}
	pinsStaged, err := stageBeside(pinsFile, source)
	if err != nil {
		return errors.Join(err, os.Remove(baselineStaged))
	}
	if err := rename(baselineStaged, baselineFile); err != nil {
		return errors.Join(err, os.Remove(baselineStaged), os.Remove(pinsStaged))
	}
	if err := rename(pinsStaged, pinsFile); err != nil {
		return errors.Join(err, os.Remove(pinsStaged), os.WriteFile(baselineFile, original, 0o600))
	}
	return nil
}

// stageBeside writes content to a new file beside path, with path's mode, so a
// rename puts it in place whole. The leading dot keeps it out of a build.
func stageBeside(path string, content []byte) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	staged, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return "", err
	}
	_, writeErr := staged.Write(content)
	if err := errors.Join(writeErr, staged.Chmod(info.Mode().Perm()), staged.Close()); err != nil {
		return "", errors.Join(err, os.Remove(staged.Name()))
	}
	return staged.Name(), nil
}

// baselineHolding is the baseline's header comments, then the kept lines.
func baselineHolding(kept []Waiver) []byte {
	var out bytes.Buffer
	for line := range strings.SplitSeq(baselineText, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		out.WriteString(line + "\n")
	}
	for _, w := range kept {
		out.WriteString(w.line() + "\n")
	}
	return out.Bytes()
}
