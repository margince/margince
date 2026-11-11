// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMinimumWriterGrantsCanStageAndRedeemTheActionFamilies(t *testing.T) {
	for _, tc := range []struct {
		kind, target string
		grant        principal.ObjectGrant
		extra        map[string]principal.ObjectGrant
	}{
		{"merge_tags", "tag", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"disqualify_lead", "lead", principal.ObjectGrant{Read: true, Delete: true}, nil},
		{"promote_lead", "lead", principal.ObjectGrant{Read: true, Update: true}, map[string]principal.ObjectGrant{"contact": {Create: true}}},
		{"enrich", "company", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"merge_records", "lead", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"merge_records", "contact", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"update_record", "company", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"archive_record", "contact", principal.ObjectGrant{Read: true, Update: true}, nil},
		{"send_email", "activity", principal.ObjectGrant{Read: true, Create: true}, map[string]principal.ObjectGrant{"contact": {Read: true}}},
		{"invite_meeting", "contact", principal.ObjectGrant{Read: true}, map[string]principal.ObjectGrant{"activity": {Create: true}}},
	} {
		t.Run(tc.kind+"/"+tc.target, func(t *testing.T) {
			e := setupStaging(t)
			ctx := e.asPassport(e.seedPassport(t))
			actor, ok := principal.Actor(ctx)
			if !ok {
				t.Fatal("no proposer")
			}
			objects := map[string]principal.ObjectGrant{tc.target: tc.grant}
			maps.Copy(objects, tc.extra)
			actor.Permissions = grantsFor(objects)
			ctx = principal.WithActor(ctx, actor)
			e.svc.WithEffectActor(func(context.Context, pgx.Tx, StagedBy) (principal.Principal, error) { return actor, nil })
			in := StageInput{Kind: tc.kind, TargetType: tc.target, TargetID: e.authoritySubject(t, tc.target), ProposedChange: []byte(`{}`), DiffHash: tc.kind + "-" + tc.target}
			if tc.kind == "merge_records" || tc.kind == "merge_tags" {
				in.CoTargetType = tc.target
				in.CoTargetID = e.authoritySubject(t, tc.target)
			}
			id, _, err := e.svc.StageAgentCall(ctx, in)
			if err != nil {
				t.Fatalf("staging with writer grants: %v", err)
			}
			if _, err := e.svc.Decide(e.asHumanWith(actor.Permissions), id, true, nil); err != nil {
				t.Fatalf("deciding with writer grants: %v", err)
			}
			if _, err := e.svc.RedeemRecords(ctx, id, in.Kind, in.DiffHash); err != nil {
				t.Fatalf("redeeming the unchanged call: %v", err)
			}
			if got := e.count(t, `SELECT count(*) FROM approval WHERE id=$1 AND consumed_at IS NOT NULL`, id); got != 1 {
				t.Fatal("redemption did not consume exactly one approval")
			}
		})
	}
}

func (e *stagingEnv) authoritySubject(t *testing.T, entity string) ids.UUID {
	t.Helper()
	switch entity {
	case "company":
		return e.seedCompany(t)
	case "tag":
		return e.seedTag(t, ids.NewV7().String())
	case "activity":
		return e.seedNote(t, "workspace")
	}
	id := ids.NewV7()
	var err error
	switch entity {
	case "contact":
		_, err = e.owner.Exec(context.Background(), `INSERT INTO contact (id,full_name,source,captured_by,owner_id) VALUES ($1,'Writer subject','manual','human:fixture',$2)`, id, e.rep)
	case "lead":
		_, err = e.owner.Exec(context.Background(), `INSERT INTO lead (id,full_name,source,captured_by,owner_id) VALUES ($1,'Writer subject','manual','human:fixture',$2)`, id, e.rep)
	default:
		t.Fatalf("unknown subject %s", entity)
	}
	if err != nil {
		t.Fatal(err)
	}
	return id
}
