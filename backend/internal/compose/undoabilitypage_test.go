// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The page decides whether to draw a restore button; the write decides whether
// to honour it. A port the write consults and the page does not is a button
// drawn enabled over a refusal — the reader presses it and gets a 409, which is
// exactly what happened to ExternallyGoverned.
//
// Two halves, because the page's evaluator is a COPY of the write's with four
// ports replaced. Checking only that the copy's fields are non-nil proves
// nothing: the copy carries every port across whether or not the page derived
// it, so the check passes with any of the four assignments deleted. The first
// half therefore asks what each derived port ANSWERS, against page facts
// chosen so the write-time port cannot produce them by accident; the second
// keeps the shape check for a port that is neither derived nor inherited.
func TestTheAdvisoryPathAnswersFromThePageFacts(t *testing.T) {
	// Every write-time port records that it ran and answers the ZERO value —
	// the opposite of each page fact below, so a derived port that fell back
	// to it is a wrong answer and not merely a different route to a right one.
	var ranAtWriteTime []string
	binding := Evaluator{}
	v := reflect.ValueOf(&binding).Elem()
	for i := range v.NumField() {
		field, name := v.Field(i), v.Type().Field(i).Name
		if field.Kind() != reflect.Func {
			t.Fatalf("Evaluator.%s is not a port; this test only knows how to fill ports", name)
		}
		field.Set(reflect.MakeFunc(field.Type(), func([]reflect.Value) []reflect.Value {
			ranAtWriteTime = append(ranAtWriteTime, name)
			out := make([]reflect.Value, field.Type().NumOut())
			for j := range out {
				out[j] = reflect.Zero(field.Type().Out(j))
			}
			return out
		}))
	}

	row := pageRow{AuditRow: AuditRow{ID: ids.MustParse("01950000-0000-7000-8000-00000000beef")}, behindErasure: true}
	shared := recordFacts{archived: true, writable: false}
	undone := map[string]bool{row.ID.String(): true}
	advisory := advisoryEvaluator(binding, shared, row, undone)

	ctx := context.Background()
	archived, err := advisory.Archived(ctx, nil, "person", row.ID)
	if err != nil || !archived {
		t.Errorf("Archived answered (%v, %v), want the page's own fact (true, nil)", archived, err)
	}
	if err := advisory.Writable(ctx, nil, "person", row.ID); !errors.Is(err, errRecordNotWritable) {
		t.Errorf("Writable answered %v, want the page's own fact %v", err, errRecordNotWritable)
	}
	behind, err := advisory.BehindErasure(ctx, nil, row.AuditRow)
	if err != nil || !behind {
		t.Errorf("BehindErasure answered (%v, %v), want the page's own fact (true, nil)", behind, err)
	}
	alreadyUndone, err := advisory.AlreadyUndone(ctx, nil, row.AuditRow)
	if err != nil || !alreadyUndone {
		t.Errorf("AlreadyUndone answered (%v, %v), want the page's own fact (true, nil)", alreadyUndone, err)
	}

	// The page reads its rows in one query. A derived port that reached the
	// write-time one is a query PER ROW as well as a wrong answer, and the
	// wrong answer is what a reader sees.
	if len(ranAtWriteTime) > 0 {
		t.Errorf("the page answered %v through the write-time port instead of the facts it already holds: "+
			"one query per row, and the row's own fact ignored", ranAtWriteTime)
	}

	// Whatever the page does NOT derive it inherits, and a port left unbound
	// is a branch the write asks and the page skips — the ExternallyGoverned
	// shape. Derived from the struct, so a new port is covered the day it
	// is added rather than the day someone remembers this test.
	bound := reflect.ValueOf(advisory)
	for i := range bound.NumField() {
		if bound.Field(i).IsNil() {
			t.Errorf("the page leaves %s unbound while the write binds it: every entry would render "+
				"undoable and refuse on press", bound.Type().Field(i).Name)
		}
	}
}
