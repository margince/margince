// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// Work adds website inquiry facts to Worklist tasks.
type Work struct {
	db          *database.DB
	contacts    *contacts.Store
	store       *activities.Store
	assignments *automation.InquiryService
}

// NewWork reads assignments through routing's inquiry service.
func NewWork(db *database.DB, routing Routing) *Work {
	return &Work{db: db, contacts: contacts.NewStore(db), store: activities.NewStore(db), assignments: automation.NewInquiryService(db, routing)}
}

// ForTasks gives the inquiry facts of each task filed from a website
// submission. Other tasks have no entry.
func (w *Work) ForTasks(ctx context.Context, tasks []crmcontracts.Activity) (map[ids.UUID]*crmcontracts.WebsiteInquiryWork, error) {
	out := map[ids.UUID]*crmcontracts.WebsiteInquiryWork{}
	taskIDs := make([]ids.ActivityID, 0, len(tasks))
	submissions := map[ids.UUID]string{}
	keys := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.SourceSystem == nil || task.SourceId == nil {
			continue
		}
		key, known := provenance.WebsiteInquirySubmission(*task.SourceSystem, *task.SourceId)
		if !known {
			continue
		}
		id := ids.UUID(task.Id)
		submissions[id] = key
		taskIDs = append(taskIDs, ids.From[ids.ActivityKind](id))
		keys = append(keys, key)
	}
	if len(taskIDs) == 0 {
		return out, nil
	}
	assignments, err := w.assignments.Assignments(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	kind, system, limit := "note", provenance.WebFormSource, len(keys)
	notes, _, err := w.store.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind, SourceSystem: &system, SourceIDs: keys, ReadableOnly: true, Limit: &limit})
	if err != nil {
		return nil, err
	}
	byKey := map[string]crmcontracts.Activity{}
	for _, note := range notes {
		if note.SourceId != nil {
			byKey[*note.SourceId] = note
		}
	}
	for id, key := range submissions {
		assignment := assignments[id]
		facts := &crmcontracts.WebsiteInquiryWork{}
		if assignment.Reason != "" {
			facts.AssignmentReason = &assignment.Reason
		}
		if note, visible := byKey[key]; visible && note.WebForm != nil {
			facts.ReceivedAt = &note.CreatedAt
			facts.SourceActivityId = &note.Id
			facts.SourceLabel = note.Subject
			facts.RequestType = inquiryRequestType(note.WebForm, assignment.RequestTypeField)
		}
		out[id] = facts
	}
	if err := w.owners(ctx, tasks, byKey, out); err != nil {
		return nil, err
	}
	return out, nil
}

func inquiryRequestType(sent *crmcontracts.WebFormSubmission, field *string) *string {
	if field == nil || sent.ValidatedPicklists == nil {
		return nil
	}
	valid := false
	for _, name := range *sent.ValidatedPicklists {
		valid = valid || name == *field
	}
	if !valid {
		return nil
	}
	var values []string
	for _, extra := range sent.Extra {
		if strings.EqualFold(extra.Name, *field) {
			values = append(values, extra.Value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	value := strings.Join(values, ";")
	return &value
}
