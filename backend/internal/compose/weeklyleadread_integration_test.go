// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWithheldLeadWeeksFinishTheirOptionalJobWork(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	e.WsExec(t, `UPDATE role SET permissions = permissions #- '{objects,lead}' WHERE key='manager'`)
	w := teamSnapshotWorker(e)
	probe := &weeklyMeasurementProbe{t: t, env: e}
	w.narrator, w.learner = probe, probe
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM weekly_review WHERE leads_withheld AND narrated_at IS NOT NULL`); got != 3 {
		t.Fatalf("completed withheld weeks=%d, want 3", got)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_review'`)
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_review'`); got != audits {
		t.Fatalf("second tick changed weekly audits from %d to %d", audits, got)
	}
	if probe.calls != 0 {
		t.Fatalf("withheld facts reached a model %d times", probe.calls)
	}
}

func TestAJobFallbackPreservesNarrativeHiddenFromItsReader(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	review, _, err := w.engine.AssembleFor(ctx, teamJobClock)
	if err != nil {
		t.Fatal(err)
	}
	const sentence = "One lead arrived."
	if err := w.engine.Narrate(ctx, review.ID, sentence, teamJobClock); err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "lead")
	denied := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	hidden, err := w.engine.LatestReview(denied, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Narrative != "" || hidden.NarratedAt != nil {
		t.Fatal("fixture narrative was not hidden")
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_review'`)
	w.narrator = &weeklyMeasurementProbe{t: t, env: e}
	w.enrichReview(denied, hidden, teamJobClock)
	restored, err := w.engine.LatestReview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Narrative != sentence || restored.NarratedAt == nil {
		t.Fatal("fallback erased a previously recorded narrative")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_review'`); got != audits {
		t.Fatal("fallback audited a no-op")
	}
}

func TestTeamSnapshotPrefersALeadWhoMayMeasureLeadFigures(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	w := teamSnapshotWorker(e)
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		if _, _, err := w.engine.AssembleFor(e.As(rep, []ids.UUID{e.Team1}, integration.AdminPerms), teamJobClock); err != nil {
			t.Fatal(err)
		}
	}
	lower, higher := e.Rep1, e.Rep2
	if lower.String() > higher.String() {
		lower, higher = higher, lower
	}
	e.WsExec(t, `INSERT INTO role (key,name,permissions) SELECT 'manager_no_leads','Team Lead without leads', permissions #- '{objects,lead}' FROM role WHERE key='manager'`)
	e.WsExec(t, `DELETE FROM role_assignment WHERE user_id=$1`, lower)
	e.WsExec(t, `INSERT INTO role_assignment (role_id,user_id) SELECT id,$1 FROM role WHERE key='manager_no_leads'`, lower)
	if err := w.snapshotTeam(e.Admin(), e.WS, liveTeam{id: e.Team1, name: "Sales", candidates: []ids.UUID{lower, higher}}, teamJobClock); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM team_weekly_review WHERE team_id=$1 AND NOT leads_withheld AND reps_counted=2`, e.Team1); got != 1 {
		t.Fatal("first candidate's missing lead permission permanently hid the team's measured figures")
	}
}
