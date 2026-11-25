// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package exttest_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/exttest"
)

var errType = reflect.TypeFor[error]()

// callEvery calls each method of v with zero arguments, a live context first,
// and returns the error each one answered.
func callEvery(ctx context.Context, v reflect.Value) map[string]error {
	answers := map[string]error{}
	for i := range v.NumMethod() {
		m := v.Method(i)
		args := make([]reflect.Value, m.Type().NumIn())
		for j := range args {
			args[j] = reflect.Zero(m.Type().In(j))
		}
		if len(args) > 0 && m.Type().In(0) == reflect.TypeFor[context.Context]() {
			args[0] = reflect.ValueOf(ctx)
		}
		out := m.Call(args)
		if last := out[len(out)-1]; last.Type() == errType && !last.IsNil() {
			answers[v.Type().Method(i).Name], _ = last.Interface().(error)
		}
	}
	return answers
}

func TestEveryPortMethodRefusesLikeAUnitThatHoldsNothing(t *testing.T) {
	t.Parallel()
	rt := reflect.ValueOf(exttest.Runtime{})
	for i := range rt.NumMethod() {
		m := rt.Type().Method(i)
		out := rt.Method(i).Type()
		if out.NumIn() != 0 || out.NumOut() != 1 || out.Out(0).Kind() != reflect.Interface {
			continue
		}
		if m.Name == "Secrets" {
			continue
		}
		port := rt.Method(i).Call(nil)[0].Elem()
		for name, err := range callEvery(t.Context(), port) {
			if !errors.Is(err, extension.ErrAttendedRead) && !errors.Is(err, extension.ErrNotDeclared) {
				t.Errorf("%s.%s answered %v, want a refusal", m.Name, name, err)
			}
		}
		if got := len(callEvery(t.Context(), port)); got != port.NumMethod() {
			t.Errorf("%s: %d of %d methods refused", m.Name, got, port.NumMethod())
		}
	}
}

func TestKitSecretsAnswerNothingStored(t *testing.T) {
	t.Parallel()
	answers := callEvery(t.Context(), reflect.ValueOf(exttest.Secrets{}))
	if len(answers) != reflect.TypeFor[extension.Secrets]().NumMethod() {
		t.Fatalf("%d of the Secrets methods answered an error", len(answers))
	}
	for name, err := range answers {
		if !errors.Is(err, extension.ErrSecretNotFound) && !errors.Is(err, extension.ErrRuntimeExpired) {
			t.Errorf("Secrets.%s answered %v", name, err)
		}
	}
}
