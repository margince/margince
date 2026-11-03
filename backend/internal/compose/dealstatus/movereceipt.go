// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type moveActivityStamp struct {
	ID       crmcontracts.Id
	Version  *crmcontracts.RowVersion
	Reminder *bool
}

func activityStamp(rows []crmcontracts.Activity) *moveActivityStamp {
	if len(rows) == 0 {
		return nil
	}
	a := rows[0]
	stamp := &moveActivityStamp{ID: a.Id, Version: a.Version}
	if a.EmailSummary != nil {
		stamp.Reminder = a.EmailSummary.RequestHasReminder
	}
	return stamp
}

type moveTaskStamp struct {
	ID         ids.UUID
	Subject    string
	DueAt      *time.Time
	AssigneeID *ids.UUID
}

type moveSeatStamp struct {
	Role, Name string
	ContactID  ids.UUID
	Attachable bool
}

type moveFollowUpStamp struct {
	ID         ids.UUID
	Subject    string
	At         time.Time
	Recipients []ids.UUID
	Precision  string
}

func moveTask(f facts) *moveTaskStamp {
	if len(f.openTasks) == 0 {
		return nil
	}
	t := f.openTasks[0]
	return &moveTaskStamp{ID: t.ID, Subject: t.Subject, DueAt: t.DueAt, AssigneeID: t.AssigneeID}
}

func moveFollowUp(f facts) *moveFollowUpStamp {
	met, ok := followUpOwed(f)
	if !ok {
		return nil
	}
	return &moveFollowUpStamp{ID: met.ActivityID, Subject: met.Subject, At: met.SentAt, Recipients: met.Recipients, Precision: met.Precision}
}

func moveSeats(f facts) []moveSeatStamp {
	out := make([]moveSeatStamp, 0, len(f.seats))
	for _, seat := range f.seats {
		out = append(out, moveSeatStamp{Role: seat.Role, Name: seat.Name, ContactID: seat.ContactID, Attachable: seat.Attachable})
	}
	return out
}

func moveBasisFingerprint(f facts) (string, error) {
	var last []crmcontracts.Activity
	if contact, ok := lastContact(f); ok {
		last = append(last, contact)
	}
	encoded, err := json.Marshal(struct {
		DealID          crmcontracts.Id
		DealVersion     *crmcontracts.RowVersion
		DealName        string
		DealStatus      crmcontracts.DealStatus
		LastContact     *moveActivityStamp
		Current, Review *moveActivityStamp
		Tasks           *moveTaskStamp
		Meeting         *activities.BookedMeeting
		FollowUp        *moveFollowUpStamp
		Seats           []moveSeatStamp
		Day, Zone, Lang string
		Policy          string
	}{
		DealID: f.deal.Id, DealVersion: f.deal.Version, DealName: f.deal.Name, DealStatus: f.deal.Status,
		LastContact: activityStamp(last), Current: requestStamp(f.currentRequests), Review: requestStamp(f.requests),
		Tasks: moveTask(f), Meeting: f.nextMeeting,
		FollowUp: moveFollowUp(f), Seats: moveSeats(f),
		Day: elapsed.CalendarDay(f.now, f.loc).Format(time.DateOnly), Zone: f.loc.String(), Lang: f.lang,
		Policy: ProjectionVersion,
	})
	if err != nil {
		return "", fmt.Errorf("fingerprint the current recommendation basis: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func assessMove(f facts, mv crmcontracts.DealStatusCardMove) (crmcontracts.DealStatusCardMove, error) {
	if mv.Action == ActionNone {
		return mv, nil
	}
	if mv.Recommendation == nil {
		return crmcontracts.DealStatusCardMove{}, errors.New("recommendation has no registered rule")
	}
	digest, err := moveBasisFingerprint(f)
	if err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	receipt := *mv.Recommendation
	receipt.DealId, receipt.PolicyVersion = f.deal.Id, ProjectionVersion
	receipt.BasisFingerprint, receipt.CheckedAt = digest, f.now
	mv.Recommendation = &receipt
	if mv.Action == ActionCreateTask && mv.Arguments != nil {
		args := semanticArguments(*mv.Arguments)
		args["recommendation"] = receipt
		mv.Arguments = &args
	}
	return mv, nil
}

func readableRequest(rows []crmcontracts.Activity) (crmcontracts.Activity, bool) {
	for _, row := range rows {
		if row.Kind == crmcontracts.ActivityKindEmail && !withheld(row) {
			return row, true
		}
	}
	return crmcontracts.Activity{}, false
}

func requestStamp(rows []crmcontracts.Activity) *moveActivityStamp {
	row, found := readableRequest(rows)
	if !found {
		return nil
	}
	return activityStamp([]crmcontracts.Activity{row})
}
