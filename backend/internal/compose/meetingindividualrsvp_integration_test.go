// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func individualRSVPRecord(t *testing.T, provider, owner, first, second, answer string) connector.NormalizedRecord {
	t.Helper()
	var event map[string]any
	if provider == sourceGCal {
		event = map[string]any{
			"id": "shared-review", "status": "confirmed", "summary": "Customer review",
			"start":     map[string]string{"dateTime": "2030-01-07T10:00:00Z"},
			"organizer": map[string]string{"email": first},
			"attendees": []map[string]any{
				{"email": first, "responseStatus": "accepted", "self": owner == first},
				{"email": second, "responseStatus": answer, "self": owner == second},
				{"email": "buyer@customer.test", "responseStatus": "accepted"},
			},
		}
	} else {
		own := connector.ResponseAccepted
		if owner == second {
			own = answer
		}
		event = map[string]any{
			"id": "shared-review-" + owner, "iCalUId": "shared-review@calendar.test", "subject": "Customer review",
			"start":          map[string]string{"dateTime": "2030-01-07T10:00:00", "timeZone": "UTC"},
			"organizer":      map[string]any{"emailAddress": map[string]string{"address": first}},
			"responseStatus": map[string]string{"response": own},
			"attendees": []map[string]any{
				{"type": "required", "emailAddress": map[string]string{"address": first}, "status": map[string]string{"response": "accepted"}},
				{"type": "required", "emailAddress": map[string]string{"address": second}},
				{"type": "required", "emailAddress": map[string]string{"address": "buyer@customer.test"}, "status": map[string]string{"response": "accepted"}},
			},
		}
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var rec connector.NormalizedRecord
	if provider == sourceGCal {
		rec, err = gcal.RecordOf(raw, owner)
	} else {
		rec, err = graphcal.RecordOf(raw, owner)
	}
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestAnAttendeesOwnDeclineKeepsTheSharedMeetingAndSurvivesAColleaguesReplay(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		for _, existingOnly := range []bool{false, true} {
			mode := map[bool]string{false: "capture", true: "existing_only"}[existingOnly]
			t.Run(provider+"/"+mode, func(t *testing.T) {
				e := integration.Setup(t)
				first, second := seatAddress(t, e, e.Rep1), seatAddress(t, e, e.Rep2)
				for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
					e.GrantRole(t, seat, "admin")
					e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES($1,$2,'connected',$3)`, provider, seat, seatAddress(t, e, seat))
				}
				sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
				rec := individualRSVPRecord(t, provider, first, first, second, "accepted")
				ref, err := sink.Upsert(connectorOwnerCtx(e, e.Rep1, provider), rec)
				if err != nil {
					t.Fatal(err)
				}
				for _, answer := range []string{"declined", "accepted"} {
					incoming := individualRSVPRecord(t, provider, second, first, second, answer)
					if existingOnly {
						if err := sink.ReplayMeeting(connectorOwnerCtx(e, e.Rep2, provider), incoming); err != nil {
							t.Fatal(err)
						}
					} else {
						replay, err := sink.Upsert(connectorOwnerCtx(e, e.Rep2, provider), incoming)
						if err != nil {
							t.Fatal(err)
						}
						if replay.ID != ref.ID {
							t.Fatal("the second calendar created another meeting")
						}
					}
					if _, err := sink.Upsert(connectorOwnerCtx(e, e.Rep1, provider), rec); err != nil {
						t.Fatal(err)
					}
					if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND address=$2 AND response=$3`, ref.ID, second, answer); n != 1 {
						t.Fatalf("%s: found %d rows with the attendee's own answer after a colleague's replay", answer, n)
					}
					if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
						for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
							var responsible bool
							query := "SELECT " + storekit.MeetingResponsibilitySQL("a", "$2") + " FROM activity a WHERE id=$1"
							if err := tx.QueryRow(context.Background(), query, ref.ID, seat).Scan(&responsible); err != nil {
								return err
							}
							want := seat == e.Rep1 || answer != connector.ResponseDeclined
							if responsible != want {
								return fmt.Errorf("seat %s responsibility=%t, want %t after %s", seat, responsible, want, answer)
							}
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					from := time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)
					free, _, err := activities.NewStore(e.DB()).Availability(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.UserKind](e.Rep2), from, from.Add(time.Hour), time.Hour)
					if err != nil {
						t.Fatal(err)
					}
					if want := answer == connector.ResponseDeclined; (len(free) > 0) != want {
						t.Fatalf("free slots=%d after %s", len(free), answer)
					}
					read, err := activities.NewStore(e.DB()).GetActivity(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ActivityKind](ref.ID), storekit.LiveOnly)
					if err != nil {
						t.Fatal(err)
					}
					shown := false
					if read.AttendeeResponses != nil {
						for _, response := range *read.AttendeeResponses {
							if response.UserId != nil && ids.UUID(*response.UserId) == e.Rep2 && string(response.Response) == answer {
								shown = true
							}
						}
					}
					if answer == connector.ResponseDeclined && read.Guests != nil {
						for _, guest := range *read.Guests {
							if guest.Address != nil && *guest.Address == second && guest.Response != nil && string(*guest.Response) == answer {
								shown = true
							}
						}
					}

					if !shown {
						t.Fatalf("the activity read omitted the attendee's %s response", answer)
					}
					if status, set := readMeetingStatus(t, e, ref.ID); set {
						t.Fatalf("shared meeting status became %s", status)
					}
				}
				if n := e.WsCount(t, `SELECT count(*) FROM activity_meeting_history WHERE activity_id=$1`, ref.ID); n != 0 {
					t.Fatalf("an attendee answer wrote %d shared transitions", n)
				}
			})
		}
	}
}

func TestAnExistingMeetingReplayCreatesNothingAndCannotUseAnotherSeatsKey(t *testing.T) {
	e := integration.Setup(t)
	first := seatAddress(t, e, e.Rep1)
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		e.GrantRole(t, seat, "admin")
		e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES($1,$2,'connected',$3)`, sourceGCal, seat, seatAddress(t, e, seat))
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	rec := individualRSVPRecord(t, sourceGCal, first, first, "buyer@customer.test", "accepted")
	if err := sink.ReplayMeeting(calendarOwnerCtx(e, e.Rep1), rec); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("uncaptured replay: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system=$1 AND source_id=$2`, sourceGCal, rec.NaturalKey.SourceID); n != 0 {
		t.Fatalf("replay created %d meetings", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE source_system=$1 AND source_id=$2`, sourceGCal, rec.NaturalKey.SourceID); n != 0 {
		t.Fatalf("replay retained %d uncaptured originals", n)
	}
	ref, err := sink.Upsert(calendarOwnerCtx(e, e.Rep1), rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.ReplayMeeting(calendarOwnerCtx(e, e.Rep2), rec); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("another seat replayed the key: %v", err)
	}
	e.WsExec(t, `UPDATE activity SET archived_at=now() WHERE id=$1`, ref.ID)
	if err := sink.ReplayMeeting(calendarOwnerCtx(e, e.Rep1), rec); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("archived replay: %v", err)
	}
}

func TestAFirstDeclinedInvitationCreatesNothing(t *testing.T) {
	e := integration.Setup(t)
	owner := seatAddress(t, e, e.Rep1)
	e.GrantRole(t, e.Rep1, "admin")
	e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES($1,$2,'connected',$3)`, sourceGCal, e.Rep1, owner)
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	rec := individualRSVPRecord(t, sourceGCal, owner, "organizer@customer.test", owner, "declined")
	if _, err := sink.Upsert(calendarOwnerCtx(e, e.Rep1), rec); !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("new decline: %v", err)
	}
	for _, table := range []string{"activity", "raw_capture"} {
		if n := e.WsCount(t, "SELECT count(*) FROM "+table+" WHERE source_system=$1 AND source_id=$2", sourceGCal, rec.NaturalKey.SourceID); n != 0 {
			t.Fatalf("new decline created %d %s rows", n, table)
		}
	}
}

func TestRemovingAnAttendeesCalendarCopyKeepsTheOrganizersMeeting(t *testing.T) {
	for _, provider := range []string{sourceGCal, sourceGraphCal} {
		t.Run(provider, func(t *testing.T) {
			e := integration.Setup(t)
			first, second := seatAddress(t, e, e.Rep1), seatAddress(t, e, e.Rep2)
			for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
				e.GrantRole(t, seat, "admin")
				e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES($1,$2,'connected',$3)`, provider, seat, seatAddress(t, e, seat))
			}
			sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
			rec := individualRSVPRecord(t, provider, first, first, second, "accepted")
			ref, err := sink.Upsert(connectorOwnerCtx(e, e.Rep1, provider), rec)
			if err != nil {
				t.Fatal(err)
			}
			own := individualRSVPRecord(t, provider, second, first, second, "accepted")
			if _, err := sink.Upsert(connectorOwnerCtx(e, e.Rep2, provider), own); err != nil {
				t.Fatal(err)
			}
			if err := sink.RemoveCalendarMeeting(connectorOwnerCtx(e, e.Rep2, provider), own.NaturalKey, own.CrossDoorIdentity, time.Time{}); err != nil {
				t.Fatal(err)
			}
			if status, set := readMeetingStatus(t, e, ref.ID); set {
				t.Fatalf("attendee removal set shared status %s", status)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND user_id=$2 AND roster_seat=$2`, ref.ID, e.Rep2); n != 0 {
				t.Fatalf("removal kept %d own calendar rows", n)
			}
			if err := sink.RemoveCalendarMeeting(connectorOwnerCtx(e, e.Rep1, provider), rec.NaturalKey, rec.CrossDoorIdentity, time.Date(2030, 1, 7, 10, 0, 0, 0, time.UTC)); err != nil {
				t.Fatal(err)
			}
			if status, _ := readMeetingStatus(t, e, ref.ID); status != "canceled" {
				t.Fatalf("organizer removal left shared status %s", status)
			}
		})
	}
}
