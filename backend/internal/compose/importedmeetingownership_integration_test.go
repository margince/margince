// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Legacy importers used the manual writer's caller defaults before source attribution was applied.
func seedImporterDefaultMeeting(t *testing.T, e *integration.Env, at time.Time) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Review buyer", nil)
	row, _, err := e.Activities.LogActivity(e.As(e.Rep1, nil, integration.AdminPerms), activities.LogActivityInput{
		Kind: "meeting", Subject: new("Imported review"), OccurredAt: &at, MeetingStatus: new("booked"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(row.Id)
	e.WsExec(t, `UPDATE activity SET source_system='mirror:legacy_crm',source_id=$2,source_author_id=$3 WHERE id=$1`, id, id.String(), e.Rep2)
	return id
}

func TestImporterDefaultsDoNotAssignMeetingsOrHistoricalCredit(t *testing.T) {
	e := integration.Setup(t)
	ahead := seedImporterDefaultMeeting(t, e, laneDay.Add(time.Hour))
	past := seedImporterDefaultMeeting(t, e, laneDay.Add(-time.Hour))
	anonymous := seedImporterDefaultMeeting(t, e, laneDay.Add(2*time.Hour))
	e.WsExec(t, `UPDATE activity SET source_author_id=NULL WHERE id=$1`, anonymous)
	targets := []ids.UUID{ahead, past, anonymous}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return laneDay })
	reader := e.As(e.Rep1, nil, integration.AdminPerms)
	mine, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Queue) != 0 || mine.Summary.Total != 0 {
		t.Fatalf("importer received work: %+v", mine.Queue)
	}
	for _, selection := range []struct {
		scope string
		owner ids.UUID
		want  int
	}{
		{"all", ids.Nil, 3}, {"all", e.Rep1, 0}, {"unassigned", ids.Nil, 3},
	} {
		page, err := feed.Worklist(reader, selection.scope, "all", selection.owner, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, row := range page.Queue {
			if row.Subject == nil || !slices.Contains(targets, ids.UUID(row.Subject.Id)) {
				continue
			}
			count++
			if row.Host != nil {
				t.Fatalf("importer is still labelled host: %+v", row.Host)
			}
		}
		if count != selection.want {
			t.Fatalf("scope=%s owner=%v meetings=%d want %d", selection.scope, selection.owner, count, selection.want)
		}
	}
	for _, id := range targets {
		row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if row.HostUserId != nil {
			t.Fatal("record projection still names the importer")
		}
		if voice := row.Voice; voice != nil && voice.Author != nil && voice.Author.UserId != nil && ids.UUID(*voice.Author.UserId) == e.Rep1 {
			t.Fatal("author projection still names the importer")
		}
		if got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_meeting_history h ON h.activity_id=a.id WHERE a.id=$1 AND EXISTS (SELECT 1 FROM `+storekit.MeetingCreditedSeatsAtChangeSQL("a", "false", "h.host_id_at_change")+` WHERE credit.id=$2)`, id, e.Rep1); got != 0 {
			t.Fatal("recorded history still credits the importer")
		}

		if got := e.WsCount(t, `SELECT count(*) FROM activity a WHERE a.id=$1 AND `+storekit.MeetingCreditSQL("a", "$2"), id, e.Rep1); got != 0 {
			t.Fatal("importer still receives meeting credit")
		}
		if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND host_user_id=$2`, id, e.Rep1); got != 1 {
			t.Fatal("read changed stored attribution")
		}
	}
}

func TestExplicitAndSourceAttributedImportedHostsRemainKnown(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.Rep1, nil, integration.AdminPerms)
	host := ids.From[ids.UserKind](e.Rep1)
	for _, named := range []bool{false, true} {
		in := activities.LogActivityInput{
			Kind: "meeting", MeetingStatus: new("booked"), Subject: new("Known review"), OccurredAt: new(laneDay.Add(time.Hour)), Source: "import",
			SourceSystem: new("mirror:legacy_crm"), SourceID: new(ids.NewV7().String()), Author: storekit.SourceAuthorInput{AuthorID: &e.Rep2},
		}
		want := e.Rep2
		if named {
			in.HostUserID = &host
			want = e.Rep1
		}
		row, _, err := e.Activities.LogActivity(importer, in)
		if err != nil {
			t.Fatal(err)
		}
		if row.HostUserId == nil || ids.UUID(*row.HostUserId) != want {
			t.Fatalf("known host=%v want %v", row.HostUserId, want)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM activity a WHERE a.id=$1 AND `+storekit.MeetingResponsibilitySQL("a", "$2"), ids.UUID(row.Id), want); got != 1 {
			t.Fatal("known host lost their meeting")
		}
		if got := e.WsCount(t, `SELECT count(*) FROM activity a WHERE a.id=$1 AND `+storekit.MeetingCreditSQL("a", "$2"), ids.UUID(row.Id), want); got != 1 {
			t.Fatal("known host lost credit")
		}
		if got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_meeting_history h ON h.activity_id=a.id WHERE a.id=$1 AND `+storekit.MeetingCreditedSeatAtChangeSQL("a", "false", "h.host_id_at_change")+`=$2`, ids.UUID(row.Id), want); got != 1 {
			t.Fatal("known host lost recorded credit")
		}

	}
}

func TestSnapshotsAndNonCreationEvidenceDoNotDeclareAMeetingHost(t *testing.T) {
	e := integration.Setup(t)
	for _, action := range []string{"restore", "update"} {
		id := seedImporterDefaultMeeting(t, e, laneDay.Add(time.Hour))
		if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			_, err := storekit.AuditWithEvidence(e.Admin(), tx, action, "activity", id, map[string]any{"host_user_id": e.Rep1}, map[string]any{"host_user_id": e.Rep1}, map[string]any{storekit.ExplicitMeetingHostEvidence: e.Rep1})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if row.HostUserId != nil {
			t.Fatalf("%s snapshot declared a host", action)
		}
	}
}
