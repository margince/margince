// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
)

// The markers around the generated region of the page. Everything outside
// them is written by hand.
const (
	pageBegin = "<!-- generated:extensions:start -->"
	pageEnd   = "<!-- generated:extensions:end -->"
)

const (
	tick = "✅"
	none = "·"
)

// doorLabels are the rows of the record table, indexed by door.
var doorLabels = [doorCount]string{
	doorLand:   "**Land** it from its own system (create or update, by the id that system gave it)",
	doorMove:   "**Move** it to another stage",
	doorCreate: "**Create** it inside the unit's own transaction (`Tx.Core()`)",
	doorRead:   "**Read** and list it, as the user who asked",
	doorFind:   "**Find** it by the id from its own system",
	doorFilter: "**Filter** it by an admin's custom field",
	doorNotice: "**Tell** a member or a role about it (a notice)",
	doorHear:   "**Hear** its events",
	doorErased: "**Hear** that it was erased",
	doorMerged: "**Hear** that it was merged",
	doorButton: "**Add a button** to its page",
	doorNote:   "**Add a small note** to its page",
	doorCard:   "**Add a card** to its page",
}

func recordTable(d doors) string {
	cols := d.columns()
	var b strings.Builder
	b.WriteString("| A unit can… |")
	for _, c := range cols {
		fmt.Fprintf(&b, " %s |", title(c))
	}
	b.WriteString("\n|---|")
	b.WriteString(strings.Repeat(":-:|", len(cols)))
	b.WriteString("\n")
	for door, label := range doorLabels {
		fmt.Fprintf(&b, "| %s |", label)
		for _, c := range cols {
			cell := none
			if d[c] != nil && d[c][door] {
				cell = tick
			}
			fmt.Fprintf(&b, " %s |", cell)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func fieldTable(fields []extensionField) string {
	var b strings.Builder
	b.WriteString("| Field | What the core says it is |\n|---|---|\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "| `%s` | %s |\n", f.name, f.doc)
	}
	return b.String()
}

func slotTable(rows []slotRow) string {
	var b strings.Builder
	b.WriteString("| Slot | Kind | Where it shows |\n|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", r.name, r.kind, r.doc)
	}
	return b.String()
}

// region is the whole generated part of the page.
func region(d doors, fields []extensionField, slots []slotRow) string {
	var b strings.Builder
	b.WriteString("<!-- Written by tools/gen-extdoc from backend/pkg/extension: change the code, then run `make extdoc-gen`. -->\n\n")
	b.WriteString(recordTable(d))
	b.WriteString("\n### What else a unit can add\n\nEvery field of `Extension` that brings something to the core.\n\n")
	b.WriteString(fieldTable(fields))
	b.WriteString("\n### Slots on a core page\n\n")
	b.WriteString(slotTable(slots))
	return b.String()
}

// splice replaces the region between the markers.
func splice(page, generated string) (string, error) {
	head, rest, found := strings.Cut(page, pageBegin)
	if !found {
		return "", fmt.Errorf("gen-extdoc: %s has no %s marker", pagePath, pageBegin)
	}
	_, tail, found := strings.Cut(rest, pageEnd)
	if !found {
		return "", fmt.Errorf("gen-extdoc: %s has no %s marker after %s", pagePath, pageEnd, pageBegin)
	}
	return head + pageBegin + "\n" + generated + pageEnd + tail, nil
}
