// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import "strings"

// evidence is what a reply may cite. Only a cell grounds a sentence, since
// the reader's evidence list shows cells. A cited candidate deal or decision
// adds only the figures its own record holds.
type evidence struct {
	sources map[string]CellIn
	cells   map[string]bool
	deals   map[string]string
	order   []string
}

// evidenceOf reads each record as a source the quantity checks already
// understand. A candidate's amount is a money value. A decision's value is
// the text it was recorded with.
func evidenceOf(in Input) evidence {
	ev := evidence{sources: map[string]CellIn{}, cells: map[string]bool{}, deals: map[string]string{}}
	add := func(source CellIn) {
		if _, seen := ev.sources[source.ID]; seen {
			return
		}
		ev.sources[source.ID] = source
		ev.order = append(ev.order, source.ID)
	}
	for _, cell := range in.Cells {
		add(cell)
		ev.cells[cell.ID] = true
	}
	for _, candidate := range in.Candidates {
		_, unit, _ := strings.Cut(candidate.Amount, " ")
		add(CellIn{ID: candidate.ID, Label: candidate.Name, Value: candidate.Amount, Display: candidate.Display, Unit: unit, Coverage: "ok"})
		ev.deals[candidate.ID] = candidate.Name
	}
	for _, decision := range in.Decisions {
		recorded := strings.Join(strings.Fields(decision.Text+" "+decision.Result+" "+decision.ReviewOn), " ")
		add(CellIn{ID: decision.ID, Label: decision.Text, Value: recorded, Coverage: "ok"})
	}
	return ev
}

// quantitiesFit holds each {{id}} to what it renders as. A cell may stand
// anywhere. A deal's amount stands once, in a sentence that names that deal.
// Otherwise a model that writes {{deal id}} for the name would see the amount
// printed where the name belongs. A decision renders as no quantity.
func (ev evidence) quantitiesFit(text string) bool {
	lowered := strings.ToLower(text)
	used := map[string]bool{}
	for _, token := range factToken.FindAllStringSubmatch(text, -1) {
		id := token[1]
		if ev.cells[id] {
			continue
		}
		name, deal := ev.deals[id]
		if !deal || used[id] || name == "" || !strings.Contains(lowered, strings.ToLower(name)) {
			return false
		}
		used[id] = true
	}
	return true
}
