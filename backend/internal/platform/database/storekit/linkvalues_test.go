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
	for _, want := range []string{"FROM taggable tg", "live_tag.archived_at IS NULL", fmt.Sprintf("LIMIT %d", linkedValueCap+1), "array_agg"} {
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

func TestACappedLinkReadingSaysItWasCut(t *testing.T) {
	full := make([]string, linkedValueCap+1)
	kept, cut := capLinked(full)
	if len(kept) != linkedValueCap || !cut {
		t.Errorf("a reading one past the cap kept %d, cut=%v", len(kept), cut)
	}
	kept, cut = capLinked(full[:linkedValueCap])
	if len(kept) != linkedValueCap || cut {
		t.Errorf("a reading at the cap kept %d, cut=%v", len(kept), cut)
	}
}
