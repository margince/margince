// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"

	"golang.org/x/mod/modfile"
)

// A misplaced or misspelled selector must stop the build rather than turn an
// example into a production extension.
func validateDevelopmentMarkers(file *modfile.File) error {
	allowed := map[int]bool{}
	for _, requirement := range file.Require {
		for _, comment := range requirement.Syntax.Suffix {
			if strings.TrimSpace(strings.TrimPrefix(comment.Token, "//")) == developmentOnlyMarker {
				allowed[comment.Start.Byte] = true
			}
		}
	}
	expressions := append([]modfile.Expr{file.Syntax}, file.Syntax.Stmt...)
	for _, expression := range file.Syntax.Stmt {
		if block, ok := expression.(*modfile.LineBlock); ok {
			expressions = append(expressions, &block.LParen, &block.RParen)
			for _, line := range block.Line {
				expressions = append(expressions, line)
			}
		}
	}
	for _, expression := range expressions {
		comments := expression.Comment()
		for _, group := range [][]modfile.Comment{comments.Before, comments.Suffix, comments.After} {
			for _, comment := range group {
				if strings.Contains(strings.ToLower(comment.Token), "margince:") && !allowed[comment.Start.Byte] {
					return fmt.Errorf("line %d: put // %s at the end of its require line, with no other text", comment.Start.Line, developmentOnlyMarker)
				}
			}
		}
	}
	return nil
}
