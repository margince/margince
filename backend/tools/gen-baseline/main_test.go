// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func TestSchemaSectionDropsOnlyWhatPgDumpWritesAboutTheSession(t *testing.T) {
	dump := strings.Join([]string{
		"SET statement_timeout = 0;",
		"SET default_table_access_method = heap;",
		"SELECT pg_catalog.set_config('search_path', '', false);",
		`\restrict token`,
		"CREATE FUNCTION public.close_rooms() RETURNS void",
		"    LANGUAGE sql",
		"    AS $$",
		"UPDATE public.room",
		"SET status = 'closed'",
		"$$;",
	}, "\n")

	kept, _ := schemaSection(dump)

	if !strings.Contains(kept, "SET status = 'closed'") {
		t.Errorf("a SET line inside a function body was dropped:\n%s", kept)
	}
	for _, session := range []string{"statement_timeout", "default_table_access_method", "set_config", `\restrict`} {
		if strings.Contains(kept, session) {
			t.Errorf("pg_dump's %s line was kept:\n%s", session, kept)
		}
	}
}
