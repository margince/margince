// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Who a meeting follow-up goes to: every contact the meeting was with. They
// are read from its links and its attendee list, under the reader's scope.

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// openDeal writes an open deal the rep owns.
func (e *loadEnv) openDeal(t *testing.T) ids.UUID {
	t.Helper()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Deal A', 'open', $2, $3, $4, 'seed', 'system')`, deal, e.rep, pipeline, stage)
	return deal
}

// metOnDeal seeds a meeting the rep held three days ago, filed under the deal
// alone, as a calendar capture with no contact link leaves it.
func (e *loadEnv) metOnDeal(t *testing.T, deal ids.UUID) ids.UUID {
	t.Helper()
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, host_user_id, source, captured_by)
		VALUES ($1, 'meeting', 'Pricing review', $3::timestamptz - interval '3 days', 3600, 'held', $2, 'seed', 'system')`,
		meeting, e.rep, clocktest.Now(t))
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), meeting, deal)
	return meeting
}

// attended writes the participant row calendar capture writes for an
// attendee whose address resolved to a contact.
func (e *loadEnv) attended(t *testing.T, activity, contact ids.UUID, role string) {
	t.Helper()
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, contact_id, address, role)
		VALUES ($1, $2, $3, $4, $5)`, ids.NewV7(), activity, contact, "c-"+contact.String()+"@customer.test", role)
}

// privateContactOf writes a contact only its owner may see.
func (e *loadEnv) privateContactOf(t *testing.T, owner ids.UUID) ids.UUID {
	t.Helper()
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Attendee', $2, 'owner', 'seed', 'system')`, contact, owner)
	return contact
}

func meetingFollowUpOf(ctx context.Context, t *testing.T, s *Store, meeting ids.UUID) (AwaitingReply, bool) {
	t.Helper()
	rows, err := s.MeetingsAwaitingFollowUp(ctx, clocktest.Now(t))
	if err != nil {
		t.Fatalf("reading meeting follow-ups: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == meeting {
			return row, true
		}
	}
	return AwaitingReply{}, false
}

func sortedIDs(in ...ids.UUID) []ids.UUID {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	return out
}

func TestAMeetingFiledOnlyOnADealGoesToItsAttendees(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	deal := e.openDeal(t)
	meeting := e.metOnDeal(t, deal)
	first, second := e.buyer(t), e.buyer(t)
	e.attended(t, meeting, first, "attendee")
	e.attended(t, meeting, second, "organizer")
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, user_id, role) VALUES ($1, $2, $3, 'attendee')`,
		ids.NewV7(), meeting, e.rep)

	row, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if !found {
		t.Fatal("a meeting on an open deal with nothing sent since is not a follow-up")
	}
	if want := sortedIDs(first, second); !slices.Equal(row.Recipients, want) {
		t.Fatalf("recipients = %v, want both attendees %v", row.Recipients, want)
	}
	if row.ContactID != row.Recipients[0] || row.DealID != deal {
		t.Errorf("contact = %s, deal = %s; want the first recipient and the deal", row.ContactID, row.DealID)
	}
}

func TestTheLinkedContactsComeBeforeTheAttendeeList(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	attendee, linked := e.buyer(t), e.buyer(t)
	e.attended(t, meeting, attendee, "attendee")
	e.attended(t, meeting, linked, "attendee")
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), meeting, linked)

	row, _ := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if want := []ids.UUID{linked, attendee}; !slices.Equal(row.Recipients, want) {
		t.Fatalf("recipients = %v, want the linked contact first and each once: %v", row.Recipients, want)
	}
}

func TestAMailToAnyAttendeeSettlesTheMeeting(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	mailed, other := e.buyer(t), e.buyer(t)
	e.attended(t, meeting, mailed, "attendee")
	e.attended(t, meeting, other, "attendee")
	sent := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'thanks', $2::timestamptz - interval '1 hour', 'seed', 'system')`, sent, clocktest.Now(t))
	e.attended(t, sent, mailed, "to")

	if _, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting); found {
		t.Error("a mail addressed to one attendee after the meeting did not settle it")
	}
}

func TestAnAttendeeTheReaderCannotSeeIsNotARecipient(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	visible, hidden := e.buyer(t), e.privateContactOf(t, e.other)
	e.attended(t, meeting, visible, "attendee")
	e.attended(t, meeting, hidden, "attendee")

	row, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if !found {
		t.Fatal("a meeting with one attendee the reader may not see is not a follow-up")
	}
	if want := []ids.UUID{visible}; !slices.Equal(row.Recipients, want) {
		t.Errorf("recipients = %v, want only the attendee the reader may see %v", row.Recipients, want)
	}
}

// A mail to an attendee the reader may not see must not settle the row: a row
// vanishing would tell them that contact exists.
func TestAMailToAnAttendeeTheReaderCannotSeeDoesNotSettleIt(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	visible, hidden := e.buyer(t), e.privateContactOf(t, e.other)
	e.attended(t, meeting, visible, "attendee")
	e.attended(t, meeting, hidden, "attendee")
	sent := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'thanks', $2::timestamptz - interval '1 hour', 'seed', 'system')`, sent, clocktest.Now(t))
	e.attended(t, sent, hidden, "to")

	if _, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting); !found {
		t.Error("a mail to an attendee the reader cannot see settled the reader's follow-up")
	}
}

// A contact shared with the reader read-only cannot take a filed send, so it
// is not offered as a recipient.
func TestAReadOnlySharedAttendeeIsNotARecipient(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	shared := e.privateContactOf(t, e.other)
	e.exec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('contact', $1, 'user', $2, 'read', $3)`, shared, e.rep, e.other)
	e.attended(t, meeting, shared, "attendee")

	row, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if !found {
		t.Fatal("a meeting with a read-only shared attendee is not a follow-up")
	}
	if len(row.Recipients) != 0 {
		t.Errorf("recipients = %v, want none: the reader may read the attendee but not file a send to them", row.Recipients)
	}
}

func TestAMeetingWithNoAttendeeOnFileStillNamesItsDeal(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	deal := e.openDeal(t)
	meeting := e.metOnDeal(t, deal)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, address, role)
		VALUES ($1, $2, 'stranger@nowhere.test', 'attendee')`, ids.NewV7(), meeting)

	row, found := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if !found {
		t.Fatal("a meeting on an open deal with no attendee on file is not a follow-up")
	}
	if len(row.Recipients) != 0 || !row.ContactID.IsZero() || row.DealID != deal {
		t.Errorf("recipients = %v, contact = %s, deal = %s; want none, none and the deal",
			row.Recipients, row.ContactID, row.DealID)
	}
}

func TestAMergedAttendeeIsListedAsTheSurvivor(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	meeting := e.metOnDeal(t, e.openDeal(t))
	retired, survivor := e.buyer(t), e.buyer(t)
	e.exec(t, `UPDATE contact SET merged_into_id = $2, archived_at = now() WHERE id = $1`, retired, survivor)
	e.attended(t, meeting, retired, "attendee")

	row, _ := meetingFollowUpOf(e.followUpReader(), t, s, meeting)
	if want := []ids.UUID{survivor}; !slices.Equal(row.Recipients, want) {
		t.Errorf("recipients = %v, want the surviving contact %v", row.Recipients, want)
	}
}

func TestTheDealReadNamesOnlyThatDealsMeetings(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	deal := e.openDeal(t)
	onDeal, elsewhere := e.metOnDeal(t, deal), e.metOnDeal(t, e.openDeal(t))
	rows, err := s.MeetingsAwaitingFollowUpOnDeal(e.followUpReader(), clocktest.Now(t), deal)
	if err != nil {
		t.Fatalf("reading the deal's meeting follow-ups: %v", err)
	}
	var got []ids.UUID
	for _, row := range rows {
		got = append(got, row.ActivityID)
	}
	if !slices.Contains(got, onDeal) || slices.Contains(got, elsewhere) {
		t.Errorf("the deal's read named %v; want %s and not %s", got, onDeal, elsewhere)
	}
}
