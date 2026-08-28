// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A tier that asks for a human must reach one.
//
// A confirm-first verb is a promise with three parts, and each is a different
// file's job: the gate has to be able to PARK the refused call, the inbox has
// to be able to SHOW the parked row, and somebody has to be able to DECIDE it.
// A verb holding two of the three is worse than one holding none — it
// advertises a control that answers nothing, and the call it refuses has
// nowhere to go.
//
// For core verbs the three are constants in one module. An extension's are
// not: a unit names its own tool, its own table and its own RBAC object, so
// the promise is assembled at boot out of a declaration nobody in this
// repository wrote. These tests read the assembly rather than any unit.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/pkg/extension"
)

// confirmFirstUnit is one unit serving one 🟡 verb over its own table.
func confirmFirstUnit(t *testing.T) []mcp.Tool {
	t.Helper()
	verb := unitVerb("demo", "forget_record", extension.TierConfirmationRequired, extension.ScopeWrite)
	verb.InputSchema = json.RawMessage(`{"type":"object","required":["record_id"],
		"properties":{"record_id":{"type":"string","format":"uuid"}}}`)
	verb.Subject = extension.Subject{Arg: "record_id", Table: "ext_demo_record"}
	tools, err := buildExtensionTools([]extension.Extension{{
		Name: "demo", Version: "1.0.0",
		Tools: []extension.Tool{{Name: "forget_record", Handle: servedHandle}},
	}}, []extension.Verb{verb})
	if err != nil {
		t.Fatalf("building a served confirm-first unit: %v", err)
	}
	return tools
}

// The whole promise, end to end over the composition's own derivation: the
// tool the registry serves and the kind the inbox decides come from ONE list,
// so a verb that can stage is a verb somebody can release.
func TestAServedConfirmFirstVerbIsAKindTheInboxCanDecide(t *testing.T) {
	tools := confirmFirstUnit(t)

	if _, stageable := tools[0].(agents.Stager); !stageable {
		t.Fatal("the served tool cannot describe its staging, so the gate would refuse every call with no " +
			"approval to redeem — the dead capability this seam exists to end")
	}

	kinds := extensionStagingKinds(tools)
	if len(kinds) != 1 {
		t.Fatalf("derived %d staged kinds from one served confirm-first verb, want 1", len(kinds))
	}
	if err := approvals.RegisterExtensionKinds(kinds); err != nil {
		t.Fatalf("the kinds a served verb produces must be ones the approvals module accepts: %v", err)
	}
	t.Cleanup(func() {
		if err := approvals.RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})

	objects, err := approvals.DecisionGrantObjects(kinds[0].Verb, kinds[0].TargetTable)
	if err != nil {
		t.Fatalf("a staged call of this kind is undecidable: %v", err)
	}
	// Deciding takes the grant PERFORMING it takes — anything less puts the
	// confirm-first control point with someone who could not do the thing they
	// are releasing.
	if len(objects) != 1 || objects[0] != "ext_demo_record" {
		t.Errorf("deciding requires %v, want the unit's own declared object", objects)
	}
}

// The refusal that keeps the promise honest while a unit has not made it: a
// served 🟡 tool with no declared subject is the dead capability, and building
// the set fails closed rather than registering one.
func TestAServedConfirmFirstVerbWithNoSubjectNeverReachesTheRegistry(t *testing.T) {
	_, err := buildExtensionTools([]extension.Extension{{
		Name: "demo", Version: "1.0.0",
		Tools: []extension.Tool{{Name: "forget_record", Handle: servedHandle}},
	}}, []extension.Verb{unitVerb("demo", "forget_record", extension.TierConfirmationRequired, extension.ScopeWrite)})
	if err == nil || !strings.Contains(err.Error(), "must declare what it stages against") {
		t.Fatalf("err = %v, want the missing-subject refusal", err)
	}
}

// grantedForStaging is a caller holding exactly the unit's declared grant, so
// the argument checks below are reached rather than short-circuited.
func grantedForStaging() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:staging", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"ext_demo_record": {Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The arguments half: a call that does not name its subject is answered as the
// caller's fault, not parked as "needs approval". stageRefusedCall returns
// whatever this returns AS the answer to the call, so a soft failure here would
// tell a caller a human is looking at something nobody staged.
func TestStagingRefusesACallThatNamesNoSubject(t *testing.T) {
	stager, ok := confirmFirstUnit(t)[0].(agents.Stager)
	if !ok {
		t.Fatal("the served tool does not implement the staging seam")
	}
	for name, args := range map[string]string{
		"the subject argument absent":      `{}`,
		"a subject that is not a uuid":     `{"record_id":"not-a-uuid"}`,
		"a subject that is not text":       `{"record_id":42}`,
		"arguments that are not an object": `["record_id"]`,
	} {
		_, err := stager.StageInfo(grantedForStaging(), json.RawMessage(args))
		if err == nil {
			t.Errorf("%s: staged, and it must not", name)
			continue
		}
		var badArgs *agents.BadArgsError
		if !errors.As(err, &badArgs) {
			t.Errorf("%s: err = %v, want a caller-fixable refusal", name, err)
		}
	}
}

// The GRANT is decided before anything else, and this is what says so.
//
// Handle runs only on the approved retry, so without a check here a principal
// holding the passport scope but not the unit's object grant could park
// approval rows against records they may not touch — and a human would be
// asked to release a call its asker was never allowed to make. Asserted with
// arguments that are ALSO wrong, so the only way to pass is to refuse on the
// grant first.
func TestStagingDecidesTheGrantBeforeItReadsTheArguments(t *testing.T) {
	stager, ok := confirmFirstUnit(t)[0].(agents.Stager)
	if !ok {
		t.Fatal("the served tool does not implement the staging seam")
	}
	_, err := stager.StageInfo(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("a caller with no grant staged an approval")
	}
	var badArgs *agents.BadArgsError
	if errors.As(err, &badArgs) {
		t.Fatalf("a caller with no grant at all was told about its arguments (%v) — the grant is what "+
			"decides whether this caller may park a row here, and it must be asked first", err)
	}
}
