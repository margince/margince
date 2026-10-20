// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// draft_report through the admission gate and the seam the composed registry
// serves it with. The gate re-reads the granting human's authority, so each
// fixture human's grants are what an agent acting for them carries.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// fixtureSeats answers each fixture human's grants and teams as their own
// context holds them, with a full seat.
type fixtureSeats map[ids.UUID]principal.Principal

func (s fixtureSeats) EffectiveRBAC(_ context.Context, _, human ids.UUID) (authz.RBAC, error) {
	held, ok := s[human]
	if !ok {
		return authz.RBAC{}, apperrors.ErrNotFound
	}
	return authz.RBAC{Permissions: held.Permissions, TeamIDs: held.TeamIDs}, nil
}

func (fixtureSeats) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (s fixtureSeats) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, s.EffectiveRBAC, s.SeatType)
}

// agentDrafts is the drafts fixture with draft_report registered behind the
// admission gate.
type agentDrafts struct {
	draftsFixture
	registry *agents.Registry
	seats    fixtureSeats
}

func agentDraftsFixture(t *testing.T) agentDrafts {
	t.Helper()
	f := reportWithReader(t)
	seats := fixtureSeats{}
	for _, ctx := range []context.Context{f.human, f.reader} {
		actor, ok := principal.Actor(ctx)
		if !ok {
			t.Fatal("missing actor")
		}
		seats[actor.UserID] = actor
	}
	registry := agents.NewRegistry(nil, auth.NewGate(seats))
	agents.RegisterReportDraftTool(registry, reportDrafter(f.env.Pool))
	return agentDrafts{draftsFixture: f, registry: registry, seats: seats}
}

// agentFor is an agent principal holding a write passport lent by human.
func (f agentDrafts) agentFor(human context.Context) context.Context {
	lender, _ := principal.Actor(human)
	return principal.WithActor(human, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:report-drafts", SeatType: principal.SeatFull,
		UserID: lender.UserID, OnBehalfOf: lender.UserID, PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})
}

func (f agentDrafts) draft(ctx context.Context, t *testing.T, args map[string]any) (crmcontracts.ReportingReport, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.registry.Invoke(ctx, "draft_report", raw)
	if err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var answer struct {
		Data crmcontracts.ReportingReport `json:"data"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("the tool's answer does not decode: %v\n%s", err, out)
	}
	return answer.Data, nil
}

// An agent acting for the report's editor saves a draft. The report keeps its
// name and audience, and a reader still sees the published charts alone.
func TestAnAgentSavesADraftOnAReportItsHumanEdits(t *testing.T) {
	f := agentDraftsFixture(t)
	drafted, err := f.draft(f.agentFor(f.human), t, map[string]any{"report_id": f.report.Id, "selection": f.shorter()})
	if err != nil {
		t.Fatalf("the agent could not draft on its human's report: %v", err)
	}
	if drafted.DraftRevision == nil || *drafted.DraftRevision != 2 || drafted.Revision != 1 {
		t.Fatalf("after the agent's draft the report is at %d with draft %v, want 1 and 2", drafted.Revision, drafted.DraftRevision)
	}
	if drafted.Name != f.report.Name || drafted.Audience != f.report.Audience {
		t.Errorf("the draft moved the report to %q for %q, want %q for %q", drafted.Name, drafted.Audience, f.report.Name, f.report.Audience)
	}
	seen, err := f.service.GetReport(f.reader, ids.UUID(f.report.Id))
	if got := blocksOf(t, seen, err); !slices.Equal(got, f.selection().Blocks) || seen.DraftRevision != nil {
		t.Errorf("the reader sees %v with draft %v, want the published charts and no draft", got, seen.DraftRevision)
	}
}

// With no report named, the agent creates a report only its human sees.
func TestAnAgentsNewReportIsPrivateToItsHuman(t *testing.T) {
	f := agentDraftsFixture(t)
	created, err := f.draft(f.agentFor(f.human), t, map[string]any{"name": "Prepared by the agent", "selection": f.shorter()})
	if err != nil {
		t.Fatalf("the agent could not create a report: %v", err)
	}
	if created.Audience != "private" {
		t.Errorf("the agent's new report reads audience %q, want private", created.Audience)
	}
	if _, err := f.service.GetReport(f.human, ids.UUID(created.Id)); err != nil {
		t.Errorf("the human the agent acts for cannot open its report: %v", err)
	}
	if _, err := f.service.GetReport(f.reader, ids.UUID(created.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another rep opening the agent's new report answered %v, want not found", err)
	}
}

// An agent lent by a rep who may read the report but not edit it is refused,
// and the report holds no draft afterwards.
func TestAnAgentActingForAReaderCannotDraft(t *testing.T) {
	f := agentDraftsFixture(t)
	if _, err := f.service.GetReport(f.reader, ids.UUID(f.report.Id)); err != nil {
		t.Fatalf("the reader cannot open the report, so the refusal below proves nothing: %v", err)
	}
	if _, err := f.draft(f.agentFor(f.reader), t, map[string]any{"report_id": f.report.Id, "selection": f.shorter()}); err == nil {
		t.Fatal("an agent acting for a rep who may not edit the report saved a draft on it")
	}
	after, err := f.service.GetReport(f.human, ids.UUID(f.report.Id))
	if err != nil || after.DraftRevision != nil {
		t.Errorf("after the refusal the report holds draft %v (%v), want none", after.DraftRevision, err)
	}
}

// The tool takes no audience. A call that names one is refused whole, and the
// report keeps the audience it had.
func TestTheDraftToolCannotChangeWhoSeesAReport(t *testing.T) {
	f := agentDraftsFixture(t)
	spec, ok := f.registry.Spec("draft_report")
	if !ok {
		t.Fatal("draft_report is not registered")
	}
	if strings.Contains(string(spec.InputSchema), `"audience"`) {
		t.Errorf("draft_report's input declares an audience: %s", spec.InputSchema)
	}
	if _, err := f.draft(f.agentFor(f.human), t, map[string]any{
		"report_id": f.report.Id, "selection": f.shorter(), "audience": "private",
	}); err == nil {
		t.Fatal("a draft call naming an audience was accepted")
	}
	after, err := f.service.GetReport(f.reader, ids.UUID(f.report.Id))
	if err != nil || after.Audience != f.report.Audience || after.DraftRevision != nil {
		t.Errorf("after the refused call the reader sees audience %q with draft %v (%v), want %q and none",
			after.Audience, after.DraftRevision, err, f.report.Audience)
	}
}

// An agent does not replace a draft an editor left open: editors preview it,
// so it may be unpublished work. The editor's draft stays as they saved it.
func TestAnAgentDoesNotReplaceAnOpenDraft(t *testing.T) {
	f := agentDraftsFixture(t)
	id := ids.UUID(f.report.Id)
	saved, err := f.service.SaveReportDraft(f.human, id, f.report.Version,
		crmcontracts.ReportingReportDraftInput{Selection: f.shorter()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.draft(f.agentFor(f.human), t, map[string]any{"report_id": f.report.Id, "selection": f.selection()}); err == nil {
		t.Fatal("an agent replaced a draft an editor had open")
	}
	revisions, err := f.service.ListReportRevisions(f.human, id)
	if err != nil {
		t.Fatal(err)
	}
	draft := revisions.Data[0]
	if draft.State != "draft" || draft.Revision != *saved.DraftRevision ||
		len(draft.Selection.Blocks) != len(f.shorter().Blocks) {
		t.Errorf("the editor's draft now reads %+v, want it as they saved it", draft)
	}
}

// An agent reaches what its human reaches. A team manager who edits a team
// report they do not own can have an agent draft on it.
func TestAnAgentDraftsForAnEditorWhoDoesNotOwnTheReport(t *testing.T) {
	f := agentDraftsFixture(t)
	teamScope := f.selection()
	teamScope.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(f.env.Team1)}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
		Name: "Team review", Audience: "team", AudienceTeamId: ptrUUID(f.env.Team1), Selection: teamScope,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID = f.env.Rep3
	actor.ID = "human:" + f.env.Rep3.String()
	actor.TeamIDs = []ids.UUID{f.env.Team1}
	actor.Permissions.RowScope = principal.RowScopeTeam
	manager := principal.WithActor(f.human, actor)
	f.seats[f.env.Rep3] = actor
	opened, err := f.service.GetReport(manager, ids.UUID(report.Id))
	if err != nil || opened.CanManage == nil || !*opened.CanManage {
		t.Fatalf("the manager cannot edit the team report (%v), so the draft below proves nothing", err)
	}
	shorter := teamScope
	shorter.Blocks = shorter.Blocks[:len(shorter.Blocks)-1]
	drafted, err := f.draft(f.agentFor(manager), t, map[string]any{"report_id": report.Id, "selection": shorter})
	if err != nil {
		t.Fatalf("the manager's agent could not draft on a report the manager edits: %v", err)
	}
	if drafted.DraftRevision == nil {
		t.Error("the agent's draft names no draft revision")
	}
}
