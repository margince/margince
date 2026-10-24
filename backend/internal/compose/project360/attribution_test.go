// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package project360

import (
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// PrincipalAttribution is a declared mirror of the attribution fields of
// AuditHistoryEntry: the same JSON names, the same Go types and the same actor
// kinds, so one frontend helper reads both.
func TestPrincipalAttributionMirrorsTheAuditHistoryEntry(t *testing.T) {
	audit := reflect.TypeFor[crmcontracts.AuditHistoryEntry]()
	for field := range reflect.TypeFor[crmcontracts.PrincipalAttribution]().Fields() {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		var twin *reflect.StructField
		for candidate := range audit.Fields() {
			if strings.Split(candidate.Tag.Get("json"), ",")[0] == name {
				twin = &candidate
			}
		}
		if twin == nil {
			t.Errorf("AuditHistoryEntry has no field %q", name)
			continue
		}
		if name == "actor_type" {
			continue
		}
		if field.Type != twin.Type {
			t.Errorf("%s is %s here and %s on AuditHistoryEntry", name, field.Type, twin.Type)
		}
	}
	kinds := func(values ...string) string { return strings.Join(values, ",") }
	got := kinds(string(crmcontracts.PrincipalAttributionActorTypeAgent), string(crmcontracts.PrincipalAttributionActorTypeBuyer),
		string(crmcontracts.PrincipalAttributionActorTypeConnector), string(crmcontracts.PrincipalAttributionActorTypeHuman),
		string(crmcontracts.PrincipalAttributionActorTypeSystem))
	want := kinds(string(crmcontracts.AuditHistoryEntryActorTypeAgent), string(crmcontracts.AuditHistoryEntryActorTypeBuyer),
		string(crmcontracts.AuditHistoryEntryActorTypeConnector), string(crmcontracts.AuditHistoryEntryActorTypeHuman),
		string(crmcontracts.AuditHistoryEntryActorTypeSystem))
	if got != want {
		t.Errorf("actor kinds %s differ from AuditHistoryEntry's %s", got, want)
	}
}
