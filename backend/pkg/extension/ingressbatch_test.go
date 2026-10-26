// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestABatchOfOneSourceWithAPositionIsAccepted(t *testing.T) {
	recs := []extension.Record{aValidRecord()}
	if err := extension.ValidateBatch("dispact", recs, "page-7"); err != nil {
		t.Fatalf("ValidateBatch: %v", err)
	}
}

func TestABatchTheCoreCouldNotSaveAPositionForIsRefused(t *testing.T) {
	other := aValidRecord()
	other.System = "another"
	tooMany := make([]extension.Record, extension.MaxBatchRecords+1)
	for i := range tooMany {
		tooMany[i] = aValidRecord()
	}
	one := []extension.Record{aValidRecord()}
	cases := map[string]struct {
		recs     []extension.Record
		position extension.BatchPosition
	}{
		"no position":                     {one, ""},
		"a position that is too long":     {one, extension.BatchPosition(strings.Repeat("x", extension.MaxPositionBytes+1))},
		"a record of another source":      {[]extension.Record{aValidRecord(), other}, "p"},
		"more records than a batch holds": {tooMany, "p"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if err := extension.ValidateBatch("dispact", c.recs, c.position); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("ValidateBatch = %v, want ErrInvalid", err)
			}
		})
	}
}
