// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// The task census rejects unsupported source stamps and unresolved task kinds.

import (
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheTaskCensusFindsEmptyConstructorsAndComposedSQL(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"empty return":      `func mint() activities.LogActivityInput { return activities.LogActivityInput{} }`,
		"allocated input":   `func mint() *activities.LogActivityInput { return new(activities.LogActivityInput) }`,
		"split SQL keyword": `func mint() string { return "INS" + "ERT INTO act" + "ivity (kind) VALUES ('task')" }`,
		"shared SQL fragments": `const prefix = "INSERT INTO "
const table = "activity"
const query = prefix + table + " (kind) VALUES ('task')"
func mint() string { return query }`,
	} {
		t.Run(name, func(t *testing.T) {
			if faults := plantedAt(t, "package planted\n"+src, taskSourceWriters); len(faults) == 0 {
				t.Fatal("the unregistered producer was missing from the census")
			}
		})
	}
}

func TestAnAutomaticTaskConstructorCannotLoseItsStampBehindItsDeclaration(t *testing.T) {
	t.Parallel()
	const src = `package planted
func mint() activities.LogActivityInput {
    return activities.LogActivityInput{Kind: "task", Origin: activities.OriginSystemRemediation, Source: "system"}
}`
	declarations := maps.Clone(taskSourceWriters)
	declarations["modules/planted/planted.go:mint"] = []string{provenance.LeadSLATaskSource}
	faults := plantedAt(t, src, declarations)
	if !strings.Contains(strings.Join(faults, "\n"), "without its source_system stamp") {
		t.Fatalf("an automatic constructor lost its stamp without a finding: %v", faults)
	}
}

func TestATaskProducerCannotHideBehindAnEmptyOrUnknownDeclaration(t *testing.T) {
	t.Parallel()
	const src = `package planted
func mint() activities.LogActivityInput { return activities.LogActivityInput{Kind: "task"} }`
	for name, declaration := range map[string][]string{
		"empty":   nil,
		"unknown": {"an unregistered family"},
	} {
		t.Run(name, func(t *testing.T) {
			declarations := maps.Clone(taskSourceWriters)
			declarations["modules/planted/planted.go:mint"] = declaration
			if faults := plantedAt(t, src, declarations); len(faults) == 0 {
				t.Fatal("an invalid declaration hid a producer")
			}
		})
	}
}

func TestANonTaskDeclarationCannotHideAnActivityInsert(t *testing.T) {
	t.Parallel()
	const src = "package planted\nfunc mint() string { return `INSERT INTO activity (kind) VALUES ('task')` }"
	declarations := maps.Clone(taskSourceWriters)
	declarations["modules/planted/planted.go:mint"] = []string{writesNoTask}
	if faults := plantedAt(t, src, declarations); len(faults) == 0 {
		t.Fatal("a non-task declaration hid a SQL task producer")
	}
}

func TestTheTaskCensusDoesNotBorrowAnotherFunctionsLocalQuery(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`package planted
func other() string { query := "INSERT INTO activity (kind) VALUES ('task')"; return query }
func mint(query string) string { return query }`,
		`package planted
const query = "INSERT INTO activity (kind) VALUES ('task')"
func mint(query string) string { return query }`,
	} {
		if faults := plantedAt(t, src, taskSourceWriters); len(faults) != 0 {
			t.Fatalf("a parameter was mistaken for an activity query: %v", faults)
		}
	}
}

func TestAnAutomaticTaskStampMustNotBeNilBlankOrCleared(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"nil":              `return activities.LogActivityInput{Kind: "task", SourceSystem: nil}`,
		"blank pointer":    `source := ""; return activities.LogActivityInput{Kind: "task", SourceSystem: &source}`,
		"blank allocation": `return activities.LogActivityInput{Kind: "task", SourceSystem: new("")}`,
		"cleared field":    `in := activities.LogActivityInput{Kind: "task", SourceSystem: new("lead_sla")}; in.SourceSystem = nil; return in`,
		"cleared variable": `source := "lead_sla"; source = ""; return activities.LogActivityInput{Kind: "task", SourceSystem: &source}`,
	} {
		t.Run(name, func(t *testing.T) {
			declarations := maps.Clone(taskSourceWriters)
			declarations["modules/planted/planted.go:mint"] = []string{provenance.LeadSLATaskSource}
			if faults := plantedAt(t, "package planted\nfunc mint() activities.LogActivityInput {"+body+"}", declarations); len(faults) == 0 {
				t.Fatal("an absent source was accepted as an automatic stamp")
			}
		})
	}
}

func TestANativeDynamicKindCannotBeDeclaredNonTask(t *testing.T) {
	t.Parallel()
	const src = `package planted
func mint(kind string) activities.LogActivityInput { return activities.LogActivityInput{Kind: kind} }`
	declarations := maps.Clone(taskSourceWriters)
	declarations["modules/planted/planted.go:mint"] = []string{writesNoTask}
	if faults := plantedAt(t, src, declarations); len(faults) == 0 {
		t.Fatal("a runtime kind was accepted as a proven non-task")
	}
}

func TestTheTaskStampProofKeepsCanonicalAndCallerOwnedValues(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		source, declaration string
	}{
		"known automatic source":     {`func mint() activities.LogActivityInput { source := "lead_sla"; return activities.LogActivityInput{Kind: "task", SourceSystem: &source} }`, provenance.LeadSLATaskSource},
		"allocated automatic source": {`func mint() activities.LogActivityInput { return activities.LogActivityInput{Kind: "task", SourceSystem: new("lead_sla")} }`, provenance.LeadSLATaskSource},
		"canonical source alias": {`import "github.com/margince/margince/backend/internal/shared/kernel/provenance"
func mint() activities.LogActivityInput { source := provenance.LeadSLATaskSource; return activities.LogActivityInput{Kind: "task", SourceSystem: &source} }`, provenance.LeadSLATaskSource},
		"caller supplied source and kind": {`func mint(kind string, source *string) activities.LogActivityInput { return activities.LogActivityInput{Kind: kind, SourceSystem: source} }`, writesCallerSource},
		"manual task with nil source":     {`func mint() activities.LogActivityInput { return activities.LogActivityInput{Kind: "task", SourceSystem: nil} }`, writesHumanTask},
		"fixed note":                      {`func mint() activities.LogActivityInput { return activities.LogActivityInput{Kind: "note"} }`, writesNoTask},
		"fixed message":                   {`func mint() activities.LogActivityInput { kind := "email"; return activities.LogActivityInput{Kind: kind} }`, writesNoTask},
		"unrelated dynamic error kind":    {`func mint(kind string) activities.LogActivityInput { _ = invalid{Kind: kind}; return activities.LogActivityInput{Kind: "note"} }`, writesNoTask},
	} {
		t.Run(name, func(t *testing.T) {
			declarations := maps.Clone(taskSourceWriters)
			declarations["modules/planted/planted.go:mint"] = []string{tc.declaration}
			if faults := plantedAt(t, "package planted\n"+tc.source, declarations); len(faults) != 0 {
				t.Fatalf("valid declared producer failed: %v", faults)
			}
		})
	}
}
