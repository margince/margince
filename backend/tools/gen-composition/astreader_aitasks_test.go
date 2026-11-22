// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func aiTaskUnitSource(entries string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		AITasks: []extension.AITask{
` + entries + `
		},
	}
}
`
}

const wholeAITask = `			{
				Name:      "invoice_read",
				Prompt:    "Read the invoice and return its total.",
				Schema:    "{\"type\":\"object\"}",
				Documents: true,
			},
`

// An operator reads what a unit may ask a model to do. The schema's body stays
// in the source; the manifest says only that one exists.
func TestAITasksDeriveIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", aiTaskUnitSource(wholeAITask))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{
		`"ai_tasks"`, `"name": "invoice_read"`, `"prompt": "Read the invoice and return its total."`,
		`"documents": true`, `"schema": true`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("derived manifest misses %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, `"type":"object"`) {
		t.Errorf("the schema body reached the manifest:\n%s", s)
	}
}

// The generator refuses what boot refuses, so a unit does not pass generation
// and then fail to start.
func TestAITasksRefuseWhatValidateRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ entries, says string }{
		"a kebab name":    {strings.Replace(wholeAITask, `"invoice_read"`, `"invoice-read"`, 1), "lower-case"},
		"no prompt":       {strings.Replace(wholeAITask, `"Read the invoice and return its total."`, `""`, 1), "Prompt"},
		"a bad schema":    {strings.Replace(wholeAITask, `"{\"type\":\"object\"}"`, `"[1]"`, 1), "Schema"},
		"a task twice":    {wholeAITask + wholeAITask, "declared twice"},
		"a computed name": {strings.Replace(wholeAITask, `"invoice_read"`, `name()`, 1), "AITask.Name"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deriveSynthetic(t, "x", aiTaskUnitSource(tc.entries))
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("derive = %v, want a refusal naming %q", err, tc.says)
			}
		})
	}
}
