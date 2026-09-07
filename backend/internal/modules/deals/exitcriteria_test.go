// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The key shape is spelled twice — once as a CHECK in the migration and once
// as criterionKeyShape, so a bad key is a 422 naming the field instead of a
// 500 from the constraint. Two spellings of one rule drift, so this reads the
// migration's own pattern and holds the Go one against it.
func TestACriterionKeyRefusalMatchesTheColumnCheck(t *testing.T) {
	sql := readMigration(t)
	found := regexp.MustCompile(`key ~ '([^']+)'`).FindStringSubmatch(sql)
	if found == nil {
		t.Fatal("the migration no longer states a key shape CHECK; this gate reads it to compare")
	}
	if found[1] != criterionKeyShape.String() {
		t.Errorf("the column accepts %q and the store accepts %q; a key the column takes would 500",
			found[1], criterionKeyShape.String())
	}
}

// A key the column would refuse must be refused here first, naming the field.
func TestABadCriterionKeyNamesTheFieldItRefuses(t *testing.T) {
	for _, key := range []string{
		"", "9leads", "Upper", "has space", "trailing-dash",
		strings.Repeat("a", maxCriterionKey+1),
	} {
		err := validCriterionKey(key)
		if err == nil {
			t.Errorf("key %q was accepted; the column's CHECK would then refuse it as a 500", key)
			continue
		}
		if field, _, _ := fieldFault(t, err); field != criterionKeyField {
			t.Errorf("key %q was refused under field %q, not %q", key, field, criterionKeyField)
		}
	}
}

// The Go bound and the column's CHECK are two spellings of one rule, so the
// gate READS the migration's number rather than deriving it from the Go
// constant. Deriving it would pass while the two disagreed, which is the
// drift this test claims to catch.
func TestTheLabelBoundIsTheColumnsBound(t *testing.T) {
	sql := readMigration(t)
	found := regexp.MustCompile(`length\(label\) BETWEEN (\d+) AND (\d+)`).FindStringSubmatch(sql)
	if found == nil {
		t.Fatal("the migration no longer states a label length CHECK; this gate reads it to compare")
	}
	if got := mustAtoi(t, found[2]); got != maxCriterionLabel {
		t.Errorf("the column accepts %d characters and the store accepts %d; one of them refuses text the other takes",
			got, maxCriterionLabel)
	}
	if got := mustAtoi(t, found[1]); got != 1 {
		t.Errorf("the column's minimum label length is %d; the store refuses only the empty string", got)
	}
}

// The key bound is the same claim for the key's own length. The shape regex is
// compared separately above; this is the independent maxCriterionKey ceiling,
// which a regex comparison alone would not reach.
func TestTheKeyBoundIsTheColumnsBound(t *testing.T) {
	sql := readMigration(t)
	found := regexp.MustCompile(`key ~ '\^\[a-z\]\[a-z0-9_\]\{0,(\d+)\}\$'`).FindStringSubmatch(sql)
	if found == nil {
		t.Fatal("the migration's key CHECK no longer states a length; this gate reads it to compare")
	}
	// The pattern bounds the characters AFTER the first, so the whole key is
	// one longer than the number it carries.
	if got := mustAtoi(t, found[1]) + 1; got != maxCriterionKey {
		t.Errorf("the column accepts keys of %d characters and the store accepts %d", got, maxCriterionKey)
	}
}

// The label bound counts CHARACTERS, because the column's length() does.
// Counting bytes would refuse text the column accepts: an accented label is
// two bytes per character and would fail at half the stated limit.
func TestTheLabelBoundCountsCharactersNotBytes(t *testing.T) {
	accented := strings.Repeat("ä", maxCriterionLabel)
	if err := validCriterionLabel(accented); err != nil {
		t.Errorf("a label of exactly %d accented characters was refused: %v", maxCriterionLabel, err)
	}
	if err := validCriterionLabel(strings.Repeat("ä", maxCriterionLabel+1)); err == nil {
		t.Error("a label one character over the limit was accepted; the column would refuse it")
	}
	if err := validCriterionLabel(""); err == nil {
		t.Error("an empty label was accepted; a criterion nobody can read is not configuration")
	}
}

// Every kind the column admits must parse, and nothing else may.
func TestTheKindVocabularyIsExactlyTheColumns(t *testing.T) {
	sql := readMigration(t)
	found := regexp.MustCompile(`(?s)kind IN \(([^)]*)\)`).FindStringSubmatch(sql)
	if found == nil {
		t.Fatal("the migration no longer states a kind CHECK; this gate reads it to compare")
	}
	inColumn := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(found[1], -1) {
		inColumn[m[1]] = true
		if _, err := ParseCriterionKind(m[1]); err != nil {
			t.Errorf("the column admits kind %q and ParseCriterionKind refuses it", m[1])
		}
	}
	for _, k := range []CriterionKind{
		CriterionBuyerConfirmed, CriterionEventHeld,
		CriterionDocumentSigned, CriterionRoleIdentified, CriterionTermsAccepted, CriterionCustom,
	} {
		if !inColumn[string(k)] {
			t.Errorf("the store spells kind %q and the column would refuse it", k)
		}
	}
	if _, err := ParseCriterionKind("sentiment"); err == nil {
		t.Error("an unlisted kind parsed; the CHECK would then refuse it as a 500")
	}
}

// An update naming no field writes nothing, so no audit row claims a
// transition that never happened.
func TestAnEmptyCriterionEditWritesNothing(t *testing.T) {
	current := crmcontracts.StageExitCriterion{Label: "Buyer confirmed", Required: true}
	if patch := criterionUpdatePatch(current, UpdateCriterionInput{}); !patch.Empty() {
		t.Errorf("an edit naming nothing produced a patch touching %v", patch.After())
	}
}

// Clearing a hint and omitting it are opposite instructions that arrive as
// the same nil pointer, so SetHint is what tells them apart.
func TestOmittingAHintLeavesItAndClearingItRemovesIt(t *testing.T) {
	hint := "ask for it in writing"
	current := crmcontracts.StageExitCriterion{Hint: &hint}

	if patch := criterionUpdatePatch(current, UpdateCriterionInput{}); !patch.Empty() {
		t.Error("an edit that never mentioned the hint changed it")
	}
	patch := criterionUpdatePatch(current, UpdateCriterionInput{SetHint: true})
	if patch.Empty() {
		t.Fatal("an explicit null on the hint changed nothing; the caller was told it worked")
	}
	if got, ok := patch.After()["hint"]; !ok || got != nil {
		t.Errorf("clearing the hint wrote %v, not a null", got)
	}
}

// readMigration answers this package's exit-criteria migration as text. The tests above compare
// the store's bounds against the column's own CHECKs, so they must read the
// migration rather than restate it — a gate that restated the pattern would
// pass while the two spellings drifted.
func readMigration(t *testing.T) string {
	t.Helper()
	const slug = "a_stage_says_what_it_takes_to_leave_it"
	matches, err := filepath.Glob(filepath.Join("..", "..", "..",
		"migrations", "core", "*_"+slug+".up.sql"))
	if err != nil {
		t.Fatalf("looking for the %s migration: %v", slug, err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one migration named %s, found %d — this test reads it to compare bounds",
			slug, len(matches))
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[0], err)
	}
	return string(raw)
}

// fieldFault pulls the contract field, code and message off a typed refusal.
// values.ParseError is what this store raises for a bad input, and the HTTP
// layer turns it into the 422 that names the field.
func fieldFault(t *testing.T, err error) (field, code, message string) {
	t.Helper()
	var parse *values.ParseError
	if !errors.As(err, &parse) {
		t.Fatalf("error %v does not name the field it refuses; a client cannot show it", err)
	}
	return parse.Field, parse.Code, parse.Message
}

// mustAtoi reads a number the migration stated, failing the test rather than
// returning a zero a comparison would silently accept.
func mustAtoi(t *testing.T, raw string) int {
	t.Helper()
	n, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("the migration stated %q where this gate expected a number: %v", raw, err)
	}
	return n
}
