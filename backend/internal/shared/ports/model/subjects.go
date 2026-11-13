// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Subjects are the records whose data a model request carries, as
// Request.Subjects declares them. Nil there, or a set naming nothing, is
// undeclared, which a task that honours restrictions of processing refuses.
// A contact is named by id or by a mail address it holds. An activity, a
// company, a deal, a project and a deal room stand for every contact on or
// linked to it. A lead stands for itself.
type Subjects struct {
	Contacts   []ids.UUID
	Addresses  []string
	Activities []ids.UUID
	Companies  []ids.UUID
	Deals      []ids.UUID
	Projects   []ids.UUID
	Rooms      []ids.UUID
	Leads      []ids.UUID
	// NoContact is why a request naming nothing carries no contact's data.
	// Only AboutNoContact sets it.
	NoContact string
}

// Empty reports a set that names no record.
func (s Subjects) Empty() bool {
	return len(s.Contacts)+len(s.Addresses)+len(s.Activities)+len(s.Companies)+len(s.Deals)+len(s.Projects)+len(s.Rooms)+len(s.Leads) == 0
}

// AboutContacts declares the contacts a request carries.
func AboutContacts(contacts ...ids.UUID) *Subjects { return &Subjects{Contacts: contacts} }

// AboutActivities declares the activities a request carries.
// A zero id names no activity, so a set of only zero ids stays undeclared.
func AboutActivities(activities ...ids.UUID) *Subjects {
	named := make([]ids.UUID, 0, len(activities))
	for _, id := range activities {
		if !id.IsZero() {
			named = append(named, id)
		}
	}
	return &Subjects{Activities: named}
}

// AboutNoContact declares that a request carries no contact's data, and why.
func AboutNoContact(reason string) *Subjects { return &Subjects{NoContact: reason} }

// SubjectRestrictions answers whether any subject is under a restriction of
// processing (GDPR Art. 18), in the caller's workspace.
type SubjectRestrictions interface {
	AnyRestricted(ctx context.Context, subjects Subjects) (bool, error)
}

// ErrSubjectRestricted is a model call refused because a contact whose data it
// carries is restricted. No provider was asked; the item waits for the lift.
var ErrSubjectRestricted = errors.New("model: a contact this request carries is under a restriction of processing")

// ErrSubjectsUndeclared is a call of a task that honours restrictions whose
// request names no subjects, refused so an unwired site cannot slip past.
var ErrSubjectsUndeclared = errors.New("model: this task honours restrictions of processing and the request declares no subjects")

// SubjectsOf declares contacts and activities given as id strings, the way a
// prompt input carries them. A string that is no id names no record.
func SubjectsOf(contacts, activities []string) *Subjects {
	parsed := func(in []string) []ids.UUID {
		var out []ids.UUID
		for _, s := range in {
			if id, err := ids.Parse(s); err == nil {
				out = append(out, id)
			}
		}
		return out
	}
	return &Subjects{Contacts: parsed(contacts), Activities: parsed(activities)}
}

// String prints the subjects by value, so two requests that declare the same
// subjects print the same.
func (s *Subjects) String() string {
	if s == nil {
		return "<undeclared>"
	}
	leads := ""
	// Printed only when named, so a set without leads prints as it always did.
	if len(s.Leads) > 0 {
		leads = fmt.Sprintf(" Leads:%v", s.Leads)
	}
	return fmt.Sprintf("{Contacts:%v Addresses:%v Activities:%v Companies:%v Deals:%v Projects:%v Rooms:%v%s NoContact:%q}",
		s.Contacts, s.Addresses, s.Activities, s.Companies, s.Deals, s.Projects, s.Rooms, leads, s.NoContact)
}
