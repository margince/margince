// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"errors"
	"testing"
)

func TestAppValidate(t *testing.T) {
	good := App{Icon: IconPlug, Label: "extx.app", Group: AppGroupWork, Object: "ext_x_visit", Export: "XApp"}
	if err := good.Validate("x"); err != nil {
		t.Fatalf("a well-formed app was refused: %v", err)
	}
	for name, mutate := range map[string]func(*App){
		"an unknown icon":       func(a *App) { a.Icon = "rocket" },
		"a label with a space":  func(a *App) { a.Label = "ext x" },
		"a label outside ext":   func(a *App) { a.Label = "nav.x" },
		"an unknown group":      func(a *App) { a.Group = "settings" },
		"a lower-case export":   func(a *App) { a.Export = "xApp" },
		"another unit's object": func(a *App) { a.Object = "ext_y_visit" },
	} {
		t.Run(name, func(t *testing.T) {
			a := good
			mutate(&a)
			if err := a.Validate("x"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
