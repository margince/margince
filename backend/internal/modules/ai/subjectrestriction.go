// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A restriction of processing (GDPR Art. 18) stops the AI reading a contact's
// data. A task that honours one is refused at every Router entry point before
// any provider, cache or trace is touched.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// SubjectRestrictionPosture is a task's declared answer to a restriction:
// "honour", or "none" with the reason the task reads no contact's data.
type SubjectRestrictionPosture struct {
	Posture string
	Reason  string
}

// ErrSubjectRestricted and ErrSubjectsUndeclared are the Router's two refusals.
var (
	ErrSubjectRestricted  = model.ErrSubjectRestricted
	ErrSubjectsUndeclared = model.ErrSubjectsUndeclared
)

// WithSubjectRestrictions sets the check a router asks before serving a task
// that honours restrictions.
func (r *Router) WithSubjectRestrictions(check model.SubjectRestrictions) *Router {
	r.restrictions = check
	return r
}

// admitSubjects refuses a request of an honouring task that declares no
// subjects, or names one under a restriction. A production router (NewRouter)
// with no check wired refuses every such call; only a unit test's bare router
// carries neither, and asks nothing.
func (r *Router) admitSubjects(ctx context.Context, task Task, req model.Request) error {
	if !HonoursRestriction(task) || (r.restrictions == nil && !r.restrictionsRequired) {
		return nil
	}
	subjects := req.Subjects
	if subjects == nil {
		subjects = ambientSubjects(ctx)
	}
	if subjects == nil || (subjects.Empty() && strings.TrimSpace(subjects.NoContact) == "") {
		return fmt.Errorf("ai: task %s: %w", task, ErrSubjectsUndeclared)
	}
	if subjects.Empty() {
		return nil
	}
	if r.restrictions == nil {
		return fmt.Errorf("ai: task %s has no restriction check wired: %w", task, ErrSubjectRestricted)
	}
	restricted, err := r.restrictions.AnyRestricted(ctx, *subjects)
	if err != nil {
		return fmt.Errorf("ai: task %s: reading restrictions of processing: %w", task, err)
	}
	if restricted {
		return fmt.Errorf("ai: task %s: %w", task, ErrSubjectRestricted)
	}
	return nil
}

// ambientSubjects reads a request's subjects off the record the site named
// with WithSubject, when the request declares none. A record stands for every
// contact on or linked to it.
func ambientSubjects(ctx context.Context) *model.Subjects {
	about, ok := SubjectOf(ctx)
	if !ok {
		return nil
	}
	id := []ids.UUID{about.Ref.ID}
	switch about.Ref.Type {
	case ids.ContactID{}.EntityType():
		return &model.Subjects{Contacts: id}
	case ids.ActivityID{}.EntityType():
		return &model.Subjects{Activities: id}
	case ids.CompanyID{}.EntityType():
		return &model.Subjects{Companies: id}
	case ids.DealID{}.EntityType():
		return &model.Subjects{Deals: id}
	case ids.ProjectID{}.EntityType():
		return &model.Subjects{Projects: id}
	case ids.LeadID{}.EntityType():
		return model.AboutNoContact("a lead is not a contact, and a restriction names contacts only")
	}
	return nil
}
