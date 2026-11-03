// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Which fiscal period holds a day is computed in one place. Reporting and
// forecasting each kept their own quarter arithmetic. A change to the fiscal
// start had to land in every copy, or the copies drifted.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fiscalElapsed is the arithmetic every copy writes: a month folded modulo
// twelve. It matches the shape, whatever the operands are called.
var fiscalElapsed = regexp.MustCompile(`Month\(\)[^\n]*%\s*12\b`)

func TestTheFiscalPeriodIsComputedOnce(t *testing.T) {
	t.Parallel()
	const owner = "internal/shared/kernel/reportperiod/label.go"
	var spellings []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if fiscalElapsed.Match(body) {
			spellings = append(spellings, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module tree: %v", err)
	}
	if len(spellings) != 1 || spellings[0] != owner {
		t.Errorf("the fiscal period arithmetic is in %v, want only %s. "+
			"Ask reportperiod.Containing for the period that holds an instant, so every reader agrees", spellings, owner)
	}
}
