// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"testing"
)

// The page decides whether to draw a restore button; the write decides whether
// to honour it. A port the write consults and the page does not is a button
// drawn enabled over a refusal — the reader presses it and gets a 409, which is
// exactly what happened to ExternallyGoverned. This holds the page's evaluator
// to the write's, by shape rather than by a list anyone has to maintain: fill
// every port on the binding evaluator, derive the advisory one, and fail on any
// port the derivation dropped.
func TestTheAdvisoryPathBindsEveryPortTheWriteBinds(t *testing.T) {
	binding := Evaluator{}
	v := reflect.ValueOf(&binding).Elem()
	for i := range v.NumField() {
		field := v.Field(i)
		if field.Kind() != reflect.Func {
			t.Fatalf("Evaluator.%s is not a port; this test only knows how to fill ports",
				v.Type().Field(i).Name)
		}
		// A zero-valued function of the port's own signature: the test never
		// calls it, so what it would return does not matter — only that the
		// derivation carried it across.
		field.Set(reflect.MakeFunc(field.Type(), func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, field.Type().NumOut())
			for j := range out {
				out[j] = reflect.Zero(field.Type().Out(j))
			}
			return out
		}))
	}

	advisory := reflect.ValueOf(advisoryEvaluator(binding, recordFacts{}, pageRow{}, nil))
	for i := range advisory.NumField() {
		if advisory.Field(i).IsNil() {
			t.Errorf("the page leaves %s unbound while the write binds it: every entry would render "+
				"undoable and refuse on press", advisory.Type().Field(i).Name)
		}
	}
}
