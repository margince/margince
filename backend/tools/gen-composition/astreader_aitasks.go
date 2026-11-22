// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared AI tasks into the manifest.
//
// An operator should see what a unit may ask a model to do before enabling it.
// The manifest publishes each task's name, the prompt, and whether it takes
// documents. The answer schema stays in the source.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredAITask is one AI task as the manifest publishes it.
type declaredAITask struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	Documents bool   `json:"documents,omitempty"`
	Schema    bool   `json:"schema,omitempty"`
}

// readAITasks reads the AITasks slice literal, sorted by name, and refuses what
// the published Validate refuses so generation and boot accept the same tasks.
func (r *unitReader) readAITasks(expr ast.Expr, file *ast.File) ([]declaredAITask, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "AITasks must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredAITask, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		task, err := r.readAITask(elt, ext)
		if err != nil {
			return nil, err
		}
		if seen[task.Name] {
			return nil, r.errAt(elt, "AI task %q declared twice", task.Name)
		}
		seen[task.Name] = true
		out = append(out, task)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *unitReader) readAITask(elt ast.Expr, ext string) (declaredAITask, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "AITask")) {
		return declaredAITask{}, r.errAt(elt, "an AITasks entry must be an extension.AITask literal")
	}
	var declared extension.AITask
	for _, e := range lit.Elts {
		kv, name, err := r.keyedField(e, "AITask")
		if err != nil {
			return declaredAITask{}, err
		}
		switch name {
		case "Name":
			var s string
			s, err = r.stringLit(kv.Value, "AITask.Name")
			declared.Name = extension.AITaskName(s)
		case "Prompt":
			declared.Prompt, err = r.stringLit(kv.Value, "AITask.Prompt")
		case "Schema":
			declared.Schema, err = r.stringLit(kv.Value, "AITask.Schema")
		case "Documents":
			declared.Documents, err = r.boolLit(kv.Value, "AITask.Documents")
		default:
			err = r.errAt(kv, "AITask field %s is not derivable by this generator", name)
		}
		if err != nil {
			return declaredAITask{}, err
		}
	}
	if err := declared.Validate(); err != nil {
		return declaredAITask{}, r.errPos(lit, "%v", err)
	}
	return declaredAITask{
		Name: string(declared.Name), Prompt: declared.Prompt, Documents: declared.Documents, Schema: declared.Schema != "",
	}, nil
}
