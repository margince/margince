// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// change_reports' draft action through the admission gate, the router and the
// seam the composed registry serves it with. The gate re-reads the granting
// human's authority, so each fixture human's grants are what an agent acting
// for them carries.

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

// agentDrafts is the drafts fixture with draft_report and the router that
// fronts it registered behind the admission gate.
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
func (f agentDrafts) agentFor(t *testing.T, human context.Context) context.Context {
	t.Helper()
	lender, ok := principal.Actor(human)
	if !ok {
		t.Fatal("missing actor")
	}
	return principal.WithActor(human, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:report-drafts", SeatType: principal.SeatFull,
		UserID: lender.UserID, OnBehalfOf: lender.UserID, PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})
}

// draft calls change_reports with action draft, the listed way to the verb.
func (f agentDrafts) draft(ctx context.Context, t *testing.T, args map[string]any) (crmcontracts.ReportingReport, error) {
	t.Helper()
	routed := map[string]any{"action": "draft"}
	for name, value := range args {
		routed[name] = value
	}
	raw, err := json.Marshal(routed)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.registry.Invoke(ctx, "change_reports", raw)
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
	drafted, err := f.draft(f.agentFor(t, f.human), t, map[string]any{"report_id": f.report.Id, "selection": f.shorter()})
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
	revisions, err := f.service.ListReportRevisions(f.human, ids.UUID(f.report.Id))
	if err != nil {
		t.Fatal(err)
	}
	if author := ids.UUID(revisions.Data[0].CreatedBy); revisions.Data[0].State != "draft" || author != f.humanID(t) {
		t.Errorf("the newest revision is %q by %s, want the draft by the human the agent acts for", revisions.Data[0].State, author)
	}
}

// With no report named, the agent creates a report only its human sees. The
// human owns it, and the audit row and its event name the agent and passport.
func TestAnAgentsNewReportIsPrivateToItsHuman(t *testing.T) {
	f := agentDraftsFixture(t)
	agent := f.agentFor(t, f.human)
	created, err := f.draft(agent, t, map[string]any{
		"name": "Prepared by the agent", "description": "Where the quarter stands", "selection": f.shorter(),
	})
	if err != nil {
		t.Fatalf("the agent could not create a report: %v", err)
	}
	if created.Audience != "private" || ids.UUID(created.OwnerId) != f.humanID(t) {
		t.Errorf("the agent's new report reads audience %q owned by %s, want private and the human", created.Audience, created.OwnerId)
	}
	if created.Description == nil || *created.Description != "Where the quarter stands" {
		t.Errorf("the agent's new report states purpose %v, want the one it was given", created.Description)
	}
	if _, err := f.service.GetReport(f.human, ids.UUID(created.Id)); err != nil {
		t.Errorf("the human the agent acts for cannot open its report: %v", err)
	}
	if _, err := f.service.GetReport(f.reader, ids.UUID(created.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another rep opening the agent's new report answered %v, want not found", err)
	}

	lent, _ := principal.Actor(agent)
	var actorType, actorID string
	var passport, onBehalfOf ids.UUID
	var events int
	if err := f.env.owner.QueryRow(context.Background(), `
		SELECT a.actor_type, a.actor_id, a.passport_id, a.on_behalf_of,
		       (SELECT count(*) FROM event_outbox e WHERE e.envelope->'trace'->>'audit_log_id' = a.id::text)
		  FROM audit_log a WHERE a.entity_type = 'report_definition' AND a.entity_id = $1 AND a.action = 'create'`,
		ids.UUID(created.Id)).Scan(&actorType, &actorID, &passport, &onBehalfOf, &events); err != nil {
		t.Fatalf("reading the new report's audit row: %v", err)
	}
	if actorType != string(principal.PrincipalAgent) || actorID != lent.ID || passport != lent.PassportID || onBehalfOf != f.humanID(t) {
		t.Errorf("the audit row names %s %s, passport %s for %s; want the agent, its passport and its human",
			actorType, actorID, passport, onBehalfOf)
	}
	if events != 1 {
		t.Errorf("the audit row has %d events in the outbox, want 1", events)
	}
}

// An agent lent by a rep who may read the report but not edit it is refused,
// and the report holds no draft afterwards.
func TestAnAgentActingForAReaderCannotDraft(t *testing.T) {
	f := agentDraftsFixture(t)
	if _, err := f.service.GetReport(f.reader, ids.UUID(f.report.Id)); err != nil {
		t.Fatalf("the reader cannot open the report, so the refusal below proves nothing: %v", err)
	}
	if _, err := f.draft(f.agentFor(t, f.reader), t, map[string]any{"report_id": f.report.Id, "selection": f.shorter()}); err == nil {
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
	for _, name := range []string{"draft_report", "change_reports"} {
		spec, ok := f.registry.Spec(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if strings.Contains(string(spec.InputSchema), `"audience"`) {
			t.Errorf("%s's input declares an audience: %s", name, spec.InputSchema)
		}
	}
	if _, err := f.draft(f.agentFor(t, f.human), t, map[string]any{
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

// Publishing, discarding, sharing and archiving stay a human's. An agent with
// a write passport is refused each, even on its own human's draft.
func TestAnAgentCannotPublishOrShareAReport(t *testing.T) {
	f := agentDraftsFixture(t)
	agent := f.agentFor(t, f.human)
	drafted, err := f.draft(agent, t, map[string]any{"report_id": f.report.Id, "selection": f.shorter()})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(f.report.Id)
	for name, write := range map[string]func() error{
		"publish": func() error { _, err := f.service.PublishReportDraft(agent, id, drafted.Version); return err },
		"discard": func() error { _, err := f.service.DiscardReportDraft(agent, id, drafted.Version); return err },
		"archive": func() error { _, err := f.service.ArchiveReport(agent, id); return err },
		"share": func() error {
			_, err := f.service.UpdateReport(agent, id, drafted.Version, crmcontracts.ReportingReportInput{
				Name: f.report.Name, Audience: "private", Selection: f.selection(),
			})
			return err
		},
		"create": func() error {
			_, err := f.service.CreateReport(agent, crmcontracts.ReportingReportInput{
				Name: "Shared by the agent", Audience: "workspace", Selection: f.selection(),
			})
			return err
		},
	} {
		if err := write(); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("an agent's %s answered %v, want permission denied", name, err)
		}
	}
	after, err := f.service.GetReport(f.human, id)
	if err != nil || after.Revision != 1 || after.DraftRevision == nil || after.Audience != f.report.Audience {
		t.Errorf("after the refusals the report reads revision %d, draft %v, audience %q (%v); want it as the draft left it",
			after.Revision, after.DraftRevision, after.Audience, err)
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
	if _, err := f.draft(f.agentFor(t, f.human), t, map[string]any{"report_id": f.report.Id, "selection": f.selection()}); err == nil {
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
	drafted, err := f.draft(f.agentFor(t, manager), t, map[string]any{"report_id": report.Id, "selection": shorter})
	if err != nil {
		t.Fatalf("the manager's agent could not draft on a report the manager edits: %v", err)
	}
	if drafted.DraftRevision == nil {
		t.Error("the agent's draft names no draft revision")
	}
}

func (f agentDrafts) humanID(t *testing.T) ids.UUID {
	t.Helper()
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	return actor.UserID
}
