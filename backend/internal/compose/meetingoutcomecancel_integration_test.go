// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A meeting outcome row offers `cancel` only where the cancel is accepted. A
// live invitation refuses it, and a canceled invitation no longer holds the meeting.
func TestAMeetingOutcomeRowOffersCancelOnlyWhereTheWriteIsAccepted(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay.Add(3 * time.Hour)
	reader := e.As(e.Rep1, nil, integration.AdminPerms)
	host := ids.From[ids.UserKind](e.Rep1)
	owner := integration.OwnerConn(t)
	book := func(subject, invitation string) ids.UUID {
		customer, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Customer for " + subject})
		if err != nil {
			t.Fatalf("creating the customer for %q: %v", subject, err)
		}
		at, booked := now.AddDate(0, 0, -2), "booked"
		row, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", MeetingStatus: &booked, HostUserID: &host,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
		})
		if err != nil {
			t.Fatalf("booking %q: %v", subject, err)
		}
		if invitation != "" {
			if _, err := owner.Exec(context.Background(), `
				INSERT INTO meeting_invitation (activity_id, host_user_id, provider, calendar_id, appointment, status, management_hash, management_ref)
				VALUES ($1, $2, 'gcal', 'primary', '{}', $3, $4, 'ref')`, ids.UUID(row.Id), e.Rep1, invitation, ids.NewV7().String()); err != nil {
				t.Fatalf("inviting to %q: %v", subject, err)
			}
		}
		return ids.UUID(row.Id)
	}
	meetings := map[string]ids.UUID{
		"no invitation":       book("Booked in the app", ""),
		"live invitation":     book("Invited and confirmed", "confirmed"),
		"canceled invitation": book("Invitation withdrawn", "canceled"),
	}
	offers := cancelOffers(t, e, now)
	for name, meeting := range meetings {
		offered, read := offers[meeting]
		if !read {
			t.Fatalf("%s: no meeting_outcome row for the meeting", name)
		}
		canceled := "canceled"
		_, err := e.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](meeting), activities.UpdateActivityInput{MeetingStatus: &canceled})
		var refusal *activities.SchedulingArgumentError
		refused := errors.As(err, &refusal) && refusal.Code == "calendar_managed"
		if err != nil && !refused {
			t.Fatalf("%s: cancel: %v", name, err)
		}
		if offered == refused {
			t.Errorf("%s: the row offers cancel = %v while the write is refused = %v", name, offered, refused)
		}
	}
}

// cancelOffers reads the reader's meeting_outcome rows and whether each offers cancel.
func cancelOffers(t *testing.T, e *integration.Env, now time.Time) map[ids.UUID]bool {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(e.As(e.Rep1, nil, integration.AdminPerms), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, row := range page.Queue {
		if row.Source != crmcontracts.WorklistItemSourceMeetingOutcome {
			continue
		}
		id, err := ids.Parse(row.Id)
		if err != nil {
			t.Fatalf("row id %q: %v", row.Id, err)
		}
		out[id] = slices.Contains(row.Actions, crmcontracts.WorklistItemActionsCancel)
	}
	return out
}
