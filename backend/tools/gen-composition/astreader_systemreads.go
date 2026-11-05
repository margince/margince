// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"slices"

	"github.com/margince/margince/backend/pkg/extension"
)

// readSystemReads reads the SystemReads slice literal, refusing a value the
// published grammar refuses and one declared twice, as the boot does.
func (r *unitReader) readSystemReads(expr ast.Expr, file *ast.File) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "SystemReads must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		read, err := r.constValue(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := extension.SystemRead(read).Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if slices.Contains(out, read) {
			return nil, r.errPos(elt, "system read %q declared twice", read)
		}
		out = append(out, read)
	}
	slices.Sort(out)
	return out, nil
}
