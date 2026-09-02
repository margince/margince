// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The transport's own obligations, which the join cannot hold for it: the
// surface is human-only, and a role that wired no store must answer rather than
// dereference nil.
//
// The SUCCESS path is proven in the integration lane
// (aicertification_integration_test.go), against a real RoutingStore over a real
// database — the only place the stored binding actually exists. A unit test
// cannot reach it: a hand-built RoutingStore has no settings store behind it, so
// it is usable here only for the refusals that return before the read.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func certReq(actor principal.Principal) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/ai/certification", nil)
	ctx := principal.WithWorkspaceID(req.Context(), ids.NewV7())
	return req.WithContext(principal.WithActor(ctx, actor))
}

// An agent never reads which vendor processes the installation's correspondence,
// nor how well it does it. The contract says x-agent-access: human-only, and the
// claim has to hold at the handler rather than only in the document.
//
// The agent is deliberately GRANTED the object: the refusal must not depend on
// it lacking a grant, because a passport's scopes can admit one.
func TestAnAgentCannotReadHowWellTheModelsPerform(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()

	h.GetAiCertification(rec, certReq(principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	}))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d — an agent read the certification posture",
			rec.Code, http.StatusForbidden)
	}
}

// A role that wired no store answers 501, not a nil dereference.
func TestAnUnwiredCertificationSurfaceIsNotImplemented(t *testing.T) {
	var h aiRoutingHandlers
	rec := httptest.NewRecorder()

	h.GetAiCertification(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/certification", nil))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// The two committed trees the surface reads must both be loadable by the binary
// that embeds them. A snapshot the product cannot decode is a settings page that
// fails for every reader, and it is cheaper to learn here than there.
func TestTheEmbeddedInputsLoad(t *testing.T) {
	t.Parallel()

	inputs, err := certificationInputs()
	if err != nil {
		t.Fatalf("the committed certification inputs do not load: %v", err)
	}
	if len(inputs.sites) == 0 {
		t.Error("the site census is empty, so every job would read as unnamed")
	}
	if len(inputs.snap.Rows) == 0 {
		t.Error("the committed snapshot carries no rows, so every job would read as unchecked")
	}
	// And the join runs over them without panicking on real data — the case a
	// hand-built fixture cannot cover, because it is the shape of the tree
	// rather than of the fixture.
	view := certificationView(boundEverywhere(), inputs.sites, inputs.snap)
	if len(view.Jobs) == 0 {
		t.Error("the real census produced no jobs")
	}
}

// A record whose timestamp will not parse keeps its counts and loses only the
// date. Dropping the measurement over a formatting fault would hide a real
// result behind a field nobody reads.
func TestAnUnparseableTimestampCostsTheDateAndNotTheResult(t *testing.T) {
	t.Parallel()

	snap, err := snapshot.FromRows([]snapshot.Row{{
		Task: "draft_reply", Site: "reply", Provider: testProvider, Model: testModel,
		EnvClass: testEnv, Status: snapshot.StatusCurrent, Band: "certified",
		Runs: 9, Passed: 9, Measured: 3, RanAt: "not-a-timestamp",
	}})
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	sites := []aitasks.Site{siteOf(ai.TaskDraftReply, "reply")}
	job := jobNamed(t, certificationView(boundEverywhere(), sites, snap), ai.TaskDraftReply)

	if job.Result != resultReliable {
		t.Errorf("result = %q, want %q — the verdict does not depend on the timestamp",
			job.Result, resultReliable)
	}
	if job.Passed == nil || *job.Passed != 9 {
		t.Errorf("the counts were lost with the date: %v", job.Passed)
	}
	if job.MeasuredAt != nil {
		t.Error("an unparseable stamp was reported as a date")
	}
}
