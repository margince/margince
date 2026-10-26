// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"strings"
	"testing"
)

func TestALinkReadingSharesTheLinkTemplateOfTheClause(t *testing.T) {
	field := Field{Expr: "tg.tag_id", Type: FieldID, ReadsLink: true, Link: TagLinkTemplate("contact")}
	sql, err := LinkValuesSQL(field, func(any) int { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FROM taggable tg", "live_tag.archived_at IS NULL", "array_agg", "array_agg"} {
		if !strings.Contains(sql, want) {
			t.Errorf("reading %q lacks %q", sql, want)
		}
	}
	if strings.Contains(sql, "EXISTS") {
		t.Errorf("reading %q still tests existence", sql)
	}
}

func TestALinkReadingAppliesTheFieldsLinkScope(t *testing.T) {
	field := Field{
		Expr: "x.v",
		Link: "EXISTS (SELECT 1 FROM x WHERE x.id = t.id AND %s)",
		LinkScope: func(arg func(any) int) (string, error) {
			return "x.owner = $1", nil
		},
	}
	sql, err := LinkValuesSQL(field, func(any) int { return 1 })
	if err != nil || !strings.Contains(sql, "x.owner = $1") {
		t.Fatalf("reading %q (%v) ignores the link scope", sql, err)
	}
}

func TestATemplateThatIsNotAnExistsCannotBeRead(t *testing.T) {
	if _, err := LinkValuesSQL(Field{Expr: "x.v", Link: "NOT EXISTS (SELECT 1 FROM x WHERE %s)"}, func(any) int { return 1 }); err == nil {
		t.Fatal("a template the reading cannot rewrite was accepted")
	}
}

func TestACappedLinkReadingCountsOnlyWhatTheReaderMayOpen(t *testing.T) {
	readable := func(n int) []LinkedValue {
		out := make([]LinkedValue, n)
		for i := range out {
			label := "x"
			out[i] = LinkedValue{Value: "v", Label: &label}
		}
		return out
	}
	hidden := []LinkedValue{{Hidden: true}, {Hidden: true}}

	kept, cut := capLinked(append(readable(linkedValueCap), hidden...))
	if cut || len(kept) != linkedValueCap+len(hidden) {
		t.Errorf("50 readable and 2 hidden kept %d, cut=%v, want no cut", len(kept), cut)
	}
	kept, cut = capLinked(readable(linkedValueCap + 1))
	if !cut || len(kept) != linkedValueCap {
		t.Errorf("51 readable kept %d, cut=%v, want 50 and a cut", len(kept), cut)
	}
}

func TestALinkReadingStopsReadingPastTheScanCap(t *testing.T) {
	sql, err := LinkValuesSQL(Field{Expr: "tg.tag_id", Link: TagLinkTemplate("contact")}, func(any) int { return 1 })
	if err != nil || !strings.Contains(sql, fmt.Sprintf("LIMIT %d", linkedValueScanCap+1)) {
		t.Fatalf("reading %q (%v) reads every linked row", sql, err)
	}
	hidden := make([]LinkedValue, linkedValueScanCap+1)
	for i := range hidden {
		hidden[i] = LinkedValue{Hidden: true}
	}
	if _, cut := capLinked(hidden); !cut {
		t.Error("a scan that stopped early reported no cut")
	}
}
