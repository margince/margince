// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

import (
	"errors"
	"io/fs"
	"testing"
)

func TestOperatorCorrectionsAreNotEmbeddedStartupMigrations(t *testing.T) {
	if _, err := files.ReadDir("datafix"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("operator datafix directory reached the migration filesystem: %v", err)
	}
}
