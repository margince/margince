// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestEntrypointVarsTellARequiredReadFromAnOptionalOne(t *testing.T) {
	dir := t.TempDir()
	script := `: "${MARGINCE_DSN:?set it}"
export MARGINCE_SCHEMA_DSN="${MARGINCE_SCHEMA_DSN:-$MARGINCE_OWNER_DSN}"
pw="${MARGINCE_ADMIN_PASSWORD:-}"
: "${MARGINCE_OWNER_DSN?set it}"
`
	if err := os.WriteFile(filepath.Join(dir, "api-entrypoint.sh"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := EntrypointVars(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []EntrypointVar{
		{Name: "MARGINCE_ADMIN_PASSWORD", Script: "api-entrypoint.sh"},
		{Name: "MARGINCE_DSN", Required: true, Script: "api-entrypoint.sh"},
		{Name: "MARGINCE_OWNER_DSN", Required: true, Script: "api-entrypoint.sh"},
		{Name: "MARGINCE_SCHEMA_DSN", Script: "api-entrypoint.sh"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
