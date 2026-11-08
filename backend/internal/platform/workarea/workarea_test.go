// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workarea

import (
	"slices"
	"testing"
)

func TestTheEmbeddedTableReads(t *testing.T) {
	file := mustRead(tableJSON)
	if len(file.Operations) == 0 || len(file.Verbs) == 0 || len(Choosable()) < 2 {
		t.Fatalf("the embedded table places %d operations and %d verbs and offers %d areas",
			len(file.Operations), len(file.Verbs), len(Choosable()))
	}
}

// A verb serving records in two areas is placed per call by its record type.
func TestARecordTypePicksTheAreaOfAVerbInSeveral(t *testing.T) {
	if areas := OfVerb("create_record"); !slices.Contains(areas, "business") || len(areas) < 2 {
		t.Fatalf("create_record is placed in %v; it serves sales records and the product catalog", areas)
	}
	for recordType, want := range map[string]string{"deal": "business", "product": "workspace_settings"} {
		if got, ok := OfRecordCall("create_record", recordType); !ok || got != want {
			t.Errorf("create_record on %s is placed in %q (%v), want %q", recordType, got, ok, want)
		}
	}
	if _, ok := OfRecordCall("create_record", "no_such_type"); ok {
		t.Error("an unknown record type is placed; it should leave the verb's areas as they are")
	}
}

func TestOnlyAChoosableAreaIsOffered(t *testing.T) {
	if !IsChoosable("admin_settings") || IsChoosable("operator") || IsChoosable("agent_governance") {
		t.Fatal("the choice must offer admin settings and never operator or agent governance")
	}
	if got := NameOf("business"); got != "Sales work" {
		t.Errorf("business is named %q to a user, want Sales work", got)
	}
}

func TestACallersCopyOfAVerbsAreasIsItsOwn(t *testing.T) {
	areas := OfVerb("read_view")
	areas[0] = "changed"
	if OfVerb("read_view")[0] == "changed" {
		t.Fatal("a caller rewrote the embedded table through the slice it was handed")
	}
}
