// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestAITaskValidate(t *testing.T) {
	whole := extension.AITask{Name: "invoice_read", Prompt: "Read the invoice.", Schema: `{"type":"object"}`}
	if err := whole.Validate(); err != nil {
		t.Fatalf("a whole declaration was refused: %v", err)
	}
	cases := map[string]func(*extension.AITask){
		"upper-case name":   func(x *extension.AITask) { x.Name = "Invoice" },
		"name with a dash":  func(x *extension.AITask) { x.Name = "invoice-read" },
		"empty name":        func(x *extension.AITask) { x.Name = "" },
		"name too long":     func(x *extension.AITask) { x.Name = extension.AITaskName(strings.Repeat("a", 49)) },
		"blank prompt":      func(x *extension.AITask) { x.Prompt = "  " },
		"prompt too large":  func(x *extension.AITask) { x.Prompt = strings.Repeat("a", extension.MaxAIPromptBytes+1) },
		"schema not object": func(x *extension.AITask) { x.Schema = `[1]` },
		"schema not JSON":   func(x *extension.AITask) { x.Schema = `{` },
	}
	for name, mutate := range cases {
		x := whole
		mutate(&x)
		if err := x.Validate(); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}
