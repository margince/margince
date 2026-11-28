// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAgentNotesLeaveEveryRecordClockQuiet(t *testing.T) {
	for _, c := range []struct {
		name, kind             string
		agent, refold, restamp bool
	}{
		{"passport note without a run", "note", true, false, false},
		{"legacy agent note after refold", "note", true, true, false},
		{"member note", "note", false, false, false},
		{"a corrected writer refreshes every clock", "note", false, false, true},
		{"assistant call", "call", true, false, false},
		{"assistant meeting", "meeting", true, false, false},
		{"assistant email", "email", true, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newPrivateMailWorld(t)
			project, err := w.e.Projects.CreateProject(w.e.Admin(), projects.CreateProjectInput{Name: "Activity clock", CompanyID: ids.From[ids.CompanyKind](w.company), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			records := map[string]ids.UUID{"contact": w.contact, "company": w.company, "deal": w.deal.UUID, "project": ids.UUID(project.Id)}
			links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact}, {EntityType: "deal", EntityID: w.deal.UUID}, {EntityType: "project", EntityID: ids.UUID(project.Id)}}
			log := func(ctx context.Context, kind string, at time.Time) ids.UUID {
				t.Helper()
				subject := "Buyer follow-up"
				in := activities.LogActivityInput{Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual", Links: links}
				if kind == "email" || kind == "call" {
					in.Direction = new("inbound")
				}
				row, _, err := w.e.Activities.LogActivity(ctx, in)
				if err != nil {
					t.Fatal(err)
				}
				actor, ok := principal.Actor(ctx)
				if !ok {
					t.Fatal("missing writer")
				}
				if actor.Type == principal.PrincipalAgent && (row.RecordedByAgent == nil || !*row.RecordedByAgent) {
					t.Fatal("History must label the same activity as AI-written")
				}
				return ids.UUID(row.Id)
			}
			old := w.now.AddDate(0, 0, -65)
			log(w.e.Admin(), "call", old)
			ctx := w.e.Admin()
			if c.agent {
				actor, ok := principal.Actor(ctx)
				if !ok {
					t.Fatal("missing member")
				}
				ctx = w.e.AgentFor(t, w.e.AdminUser, nil, actor.Permissions)
			}
			recent := w.now.Add(-time.Hour)
			recentID := log(ctx, c.kind, recent)
			want := recent
			if (c.agent && c.kind == "note") || c.restamp {
				want = old
			}
			if c.restamp {
				w.e.WsExec(t, `UPDATE activity SET captured_by=$1 WHERE id=$2`, "agent:passport", recentID)
			}
			if c.refold {
				for table, id := range records {
					w.e.WsExec(t, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET last_activity_at=$1 WHERE id=$2`, recent, id)
				}
				w.e.RunDataTask(principal.SystemActing(w.e.Admin(), "system:datatask-test"), t, compose.ActivityClocksTask())
			}
			for table, id := range records {
				if n := w.e.WsCount(t, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id=$1 AND last_activity_at=$2`, id, want); n != 1 {
					t.Errorf("%s clock must remain %s", table, want)
				}
			}
		})
	}
}
