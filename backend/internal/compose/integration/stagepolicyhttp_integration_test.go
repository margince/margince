// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The transition-policy surface over real HTTP.
//
// The store's own tests hold what the rules MEAN. What these hold is that an
// admin can reach them: a governance mechanism that works in Go and answers
// nothing on the wire is, from the settings page, a feature nobody built.
//
// The distinction every test here turns on: ASKING IS NOT BEING ALLOWED.
// Writing `auto` records what an admin wants and applies nothing by itself.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// putPolicy saves one transition's rule over HTTP and answers the recorder.
func putPolicy(
	t *testing.T, e *Env, pipeline ids.PipelineID,
	body crmcontracts.SetTransitionPolicyRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the rule: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/v1/stage-automation/policies/"+pipeline.String(), bytes.NewReader(raw)).
		WithContext(e.Admin())
	req.Header.Set("Content-Type", "application/json")
	deals.NewHandlers(e.DB(), installseam.Deals()).SetTransitionPolicy(rec, req,
		crmcontracts.Id(openapi_types.UUID(pipeline.UUID)),
		crmcontracts.SetTransitionPolicyParams{})
	return rec
}

// listPolicies reads a pipeline's rules over HTTP.
func listPolicies(
	t *testing.T, e *Env, pipeline ids.PipelineID,
) (*httptest.ResponseRecorder, crmcontracts.TransitionPolicyList) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/v1/stage-automation/policies/"+pipeline.String(), nil).
		WithContext(e.Admin())
	deals.NewHandlers(e.DB(), installseam.Deals()).ListTransitionPolicies(rec, req,
		crmcontracts.Id(openapi_types.UUID(pipeline.UUID)))
	var out crmcontracts.TransitionPolicyList
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode the rules: %v", err)
		}
	}
	return rec, out
}

// A rule saved over HTTP comes back on the list, thresholds and all.
func TestATransitionRuleIsSavedAndReadBackOverHTTP(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)

	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	}); rec.Code != http.StatusOK {
		t.Fatalf("saving the rule: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec, got := listPolicies(t, e, pipeline)
	if rec.Code != http.StatusOK {
		t.Fatalf("listing the rules: status %d, body %s", rec.Code, rec.Body.String())
	}
	if len(got.Data) != 1 {
		t.Fatalf("the pipeline reports %d rules, want 1", len(got.Data))
	}
	rule := got.Data[0]
	if rule.Mode != crmcontracts.TransitionModeAuto {
		t.Errorf("the saved rule reads %q on the wire, want auto", rule.Mode)
	}
	// The product's defaults travel, so a settings page can show the bar this
	// transition is held to without knowing what the defaults are.
	if rule.MinReviewed == 0 || rule.MinObservationDays == 0 ||
		rule.CleanAcceptanceThreshold == 0 || rule.CorrectionReversalThreshold == 0 {
		t.Errorf("the rule's thresholds are absent on the wire (%+v): a reader "+
			"cannot see what this transition must clear", rule)
	}
	if rule.UndoWindowHours == 0 {
		t.Error("the undo window is absent, so a screen offering Undo cannot say " +
			"how long it lasts")
	}
	if rule.EnabledBy == nil || rule.EnabledAt == nil {
		t.Error("turning a transition on recorded nobody: who first trusted it is " +
			"the fact an auditor asks for")
	}
	if rule.SuspendedAt != nil {
		t.Error("a freshly saved rule reads as suspended")
	}
}

// Omitting a threshold KEEPS it. It does not reset it.
//
// A caller who only means to move the window must not silently restore three
// other bars an admin set deliberately.
func TestSavingARuleKeepsTheThresholdsTheCallerDidNotName(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	strict := 900

	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModePropose,
		MinReviewed: &strict,
	}); rec.Code != http.StatusOK {
		t.Fatalf("saving the strict rule: %d %s", rec.Code, rec.Body.String())
	}
	_, first := listPolicies(t, e, pipeline)
	if len(first.Data) != 1 || first.Data[0].MinReviewed != strict {
		t.Fatalf("the strict bar did not land, so the assertion below would pass "+
			"however the second save behaved: %+v", first.Data)
	}

	// A second save naming ONLY the mode.
	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	}); rec.Code != http.StatusOK {
		t.Fatalf("turning the rule on: %d %s", rec.Code, rec.Body.String())
	}

	_, after := listPolicies(t, e, pipeline)
	if len(after.Data) != 1 {
		t.Fatalf("the pipeline reports %d rules, want 1", len(after.Data))
	}
	if after.Data[0].MinReviewed != strict {
		t.Errorf("min_reviewed fell from %d to %d across a save that never "+
			"mentioned it: an admin turning a transition on silently reset a bar "+
			"somebody set", strict, after.Data[0].MinReviewed)
	}
	if after.Data[0].Mode != crmcontracts.TransitionModeAuto {
		t.Errorf("the mode reads %q, want the auto that was just asked for",
			after.Data[0].Mode)
	}
}

// A rule about stages from another pipeline is refused.
//
// The foreign keys admit the pair — each stage exists — so the row would sit
// there describing a move no deal can make.
func TestARuleNamingAStageFromAnotherPipelineIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	// A SECOND pipeline, built through the real writer. DealFixture cannot be
	// called twice — its SeedDefaults refuses a second default pipeline — and
	// an earlier version of this test did exactly that, so it never reached
	// the code it claims to test.
	other, err := e.Deals.CreatePipeline(e.Admin(), deals.CreatePipelineInput{
		Name: "Another pipeline", Position: 2,
		Stages: []deals.StageInput{
			{Name: "Elsewhere", Position: 1, Semantic: "open", WinProbability: 10},
		},
	})
	if err != nil {
		t.Fatalf("creating the second pipeline: %v", err)
	}
	if other.Stages == nil || len(*other.Stages) == 0 {
		t.Fatal("the second pipeline has no stages, so there is no foreign stage to name")
	}
	otherOpen := ids.From[ids.StageKind](ids.UUID((*other.Stages)[0].Id))
	if otherOpen == open {
		t.Fatal("the two fixtures resolved to the SAME stage, so this test would " +
			"pass against a rule that never checks the pipeline")
	}

	rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(otherOpen.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a rule was saved about a move no deal in this pipeline can make")
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("the refusal answered %d, want 404", rec.Code)
	}
}

// An unknown pipeline is a 404, not an empty list.
//
// The two look identical on screen and mean opposite things: one says nobody
// has decided about any transition yet, the other says the id is wrong.
func TestAnUnknownPipelineHasNoPolicyList(t *testing.T) {
	e := Setup(t)
	rec, _ := listPolicies(t, e, ids.From[ids.PipelineKind](ids.NewV7()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown pipeline answered %d, want 404 — an empty list reads "+
			"as a pipeline nobody has configured", rec.Code)
	}
}

// A suspension is lifted by its own verb, and an ordinary save does not clear it.
func TestOnlyTheResumeVerbLiftsASuspension(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	ref := deals.TransitionRef{PipelineID: pipeline, FromStageID: open, ToStageID: won}

	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	}); rec.Code != http.StatusOK {
		t.Fatalf("saving the rule: %d %s", rec.Code, rec.Body.String())
	}
	if err := e.Deals.SuspendForSafetyDefect(
		e.Admin(), ref, deals.DefectCrossTenant); err != nil {
		t.Fatalf("suspending: %v", err)
	}

	// An ordinary save must NOT clear it, or the safety stop is a checkbox.
	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	}); rec.Code != http.StatusOK {
		t.Fatalf("re-saving the rule: %d %s", rec.Code, rec.Body.String())
	}
	_, saved := listPolicies(t, e, pipeline)
	if len(saved.Data) != 1 || saved.Data[0].SuspendedAt == nil {
		t.Fatalf("an ordinary save cleared the suspension: the safety stop is a "+
			"checkbox, cleared by whoever edits a threshold next (%+v)", saved.Data)
	}
	if saved.Data[0].SuspendedReason == nil || *saved.Data[0].SuspendedReason == "" {
		t.Error("the suspended rule names no reason on the wire, so the screen " +
			"offering Re-enable cannot say what it is re-enabling past")
	}

	rec := httptest.NewRecorder()
	raw, err := json.Marshal(crmcontracts.TransitionRef{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
	})
	if err != nil {
		t.Fatalf("encoding the transition: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/v1/stage-automation/policies/"+pipeline.String()+"/resume",
		bytes.NewReader(raw)).WithContext(e.Admin())
	req.Header.Set("Content-Type", "application/json")
	deals.NewHandlers(e.DB(), installseam.Deals()).ResumeTransitionPolicy(rec, req,
		crmcontracts.Id(openapi_types.UUID(pipeline.UUID)))
	if rec.Code != http.StatusOK {
		t.Fatalf("resuming: %d %s", rec.Code, rec.Body.String())
	}

	var resumed crmcontracts.TransitionPolicy
	if err := json.Unmarshal(rec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("decode the resumed rule: %v", err)
	}
	if resumed.SuspendedAt != nil {
		t.Error("the rule is still suspended after its own resume verb")
	}
	// Resuming does not re-decide. The admin's mode survives a suspension, so
	// what comes back is what was actually asked for.
	if resumed.Mode != crmcontracts.TransitionModeAuto {
		t.Errorf("the resumed rule reads %q, want the auto the admin had set — "+
			"a suspension that rewrote the mode would make them re-enable "+
			"something they never turned off", resumed.Mode)
	}
}

// readOnlyPipelinePerms is a member who may LOOK at pipelines and not change
// them — the ordinary rep's shape, and the one this door has to refuse.
var readOnlyPipelinePerms = principal.Permissions{
	RowScope: principal.RowScopeAll,
	Objects:  map[string]principal.ObjectGrant{"pipeline": {Read: true}},
}

// A reader without pipeline:update cannot save a rule.
func TestSavingARuleNeedsThePipelineGrant(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)

	raw, err := json.Marshal(crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	})
	if err != nil {
		t.Fatalf("encoding the rule: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/v1/stage-automation/policies/"+pipeline.String(), bytes.NewReader(raw)).
		WithContext(e.As(e.Rep1, []ids.UUID{e.Team1}, readOnlyPipelinePerms))
	req.Header.Set("Content-Type", "application/json")
	deals.NewHandlers(e.DB(), installseam.Deals()).SetTransitionPolicy(rec, req,
		crmcontracts.Id(openapi_types.UUID(pipeline.UUID)),
		crmcontracts.SetTransitionPolicyParams{})
	if rec.Code == http.StatusOK {
		t.Fatal("a reader with no pipeline:update turned a transition on")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("the refusal answered %d, want 403", rec.Code)
	}
}

// The contract's own bounds are enforced, with a 422 naming the field.
//
// Nothing else enforces them: the router does not call the generated Valid(),
// and the column CHECKs test positivity — which is the invariant the TABLE
// needs, not the one the API publishes. Unenforced, a mode the contract does
// not define reached the store and came back a 500, and a 400-day window was
// stored with a 200 against a contract that says 365.
func TestARuleOutsideTheContractsBoundsIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	tooLong := 400
	tooManyHours := 9000
	tooHigh := 1.5

	for _, tc := range []struct {
		name string
		body crmcontracts.SetTransitionPolicyRequest
		// names is the field the refusal must point at. A 422 that says only
		// "body" is a 422 the caller cannot act on, and the threshold case
		// gets one from the column CHECK whether this handler guards it or
		// not — so the status alone would prove nothing there.
		names string
	}{
		{
			name: "a mode the contract does not define",
			body: crmcontracts.SetTransitionPolicyRequest{
				FromStageId: openapi_types.UUID(open.UUID),
				ToStageId:   openapi_types.UUID(won.UUID),
				Mode:        crmcontracts.SetTransitionPolicyRequestMode("whenever"),
			},
			names: "mode",
		},
		{
			name: "a window longer than the contract allows",
			body: crmcontracts.SetTransitionPolicyRequest{
				FromStageId: openapi_types.UUID(open.UUID),
				ToStageId:   openapi_types.UUID(won.UUID),
				Mode:        crmcontracts.SetTransitionModeAuto,
				WindowDays:  &tooLong,
			},
			names: "window_days",
		},
		{
			name: "an undo window longer than a year",
			body: crmcontracts.SetTransitionPolicyRequest{
				FromStageId:     openapi_types.UUID(open.UUID),
				ToStageId:       openapi_types.UUID(won.UUID),
				Mode:            crmcontracts.SetTransitionModeAuto,
				UndoWindowHours: &tooManyHours,
			},
			names: "undo_window_hours",
		},
		{
			name: "a threshold written as a percentage",
			body: crmcontracts.SetTransitionPolicyRequest{
				FromStageId:              openapi_types.UUID(open.UUID),
				ToStageId:                openapi_types.UUID(won.UUID),
				Mode:                     crmcontracts.SetTransitionModeAuto,
				CleanAcceptanceThreshold: &tooHigh,
			},
			names: "clean_acceptance_threshold",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := putPolicy(t, e, pipeline, tc.body)
			if rec.Code == http.StatusOK {
				t.Fatalf("%s was accepted", tc.name)
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s answered %d, want 422 naming the field — a 500 tells "+
					"a caller reading the contract that the server broke",
					tc.name, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.names) {
				t.Errorf("%s was refused without naming %s: the body says %s, "+
					"which leaves the caller to guess which of six fields is wrong",
					tc.name, tc.names, rec.Body.String())
			}
		})
	}
}

// Resuming answers the row the resume transaction committed.
//
// Read back in a second call it would need pipeline:read of its own, so an
// update-only caller could clear a suspension and then be told 403 — a
// completed mutation looking like a failed one.
func TestResumingAnswersTheRowItCommitted(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	ref := deals.TransitionRef{PipelineID: pipeline, FromStageID: open, ToStageID: won}

	if rec := putPolicy(t, e, pipeline, crmcontracts.SetTransitionPolicyRequest{
		FromStageId: openapi_types.UUID(open.UUID),
		ToStageId:   openapi_types.UUID(won.UUID),
		Mode:        crmcontracts.SetTransitionModeAuto,
	}); rec.Code != http.StatusOK {
		t.Fatalf("saving the rule: %d %s", rec.Code, rec.Body.String())
	}
	if err := e.Deals.SuspendForSafetyDefect(
		e.Admin(), ref, deals.DefectAuthorization); err != nil {
		t.Fatalf("suspending: %v", err)
	}

	resumed, err := e.Deals.ResumeTransitionPolicy(e.Admin(), ref)
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if resumed.Suspended() {
		t.Error("the rule the resume returned is still suspended, so the 200 " +
			"says one thing and the body another")
	}
	if resumed.Mode != deals.ModeAuto {
		t.Errorf("the resumed rule reads %q, want the admin's auto", resumed.Mode)
	}
	// Resuming a rule nobody suspended is a no-op that still answers the rule.
	again, err := e.Deals.ResumeTransitionPolicy(e.Admin(), ref)
	if err != nil {
		t.Fatalf("resuming an unsuspended rule answered an error: %v", err)
	}
	if again.ID != resumed.ID {
		t.Error("resuming twice answered a different rule")
	}
}
