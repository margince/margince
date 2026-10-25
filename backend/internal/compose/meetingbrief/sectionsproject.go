// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// How the brief speaks about the body of work a meeting belongs to.
//
// A delivery meeting is the case these lines exist for: months after
// close-won there is often no open deal at all, and the sections that lead the
// brief fell silent exactly when the engagement was the whole point.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// projectHeaderLine names the engagement and where it stands. The key rides
// along when the project has one, because that is the string a reader sees in
// subject lines all day and recognises faster than the name.
func (said briefCopy) projectHeaderLine(project ProjectIn) string {
	parts := []string{project.Name}
	if project.Key != "" {
		parts[0] = fmt.Sprintf("%s (%s)", project.Name, project.Key)
	}
	if project.Phase != "" {
		parts = append(parts, project.Phase)
	}
	if project.TargetEndDate != nil {
		parts = append(parts, fmt.Sprintf(said.Say(headerWords.Target), project.TargetEndDate.Format(time.DateOnly)))
	}
	return strings.Join(parts, " · ") + "."
}

func meetingLine(in Input) string {
	said := briefCopyFor(in.Language)
	subject := in.Subject
	if subject == "" {
		subject = said.Say(headerWords.Meeting)
	}
	when := in.StartsAt.Format("2006-01-02 15:04 MST")
	if in.StartsDateOnly {
		when = in.startsText()
	}
	if in.Company == "" {
		return fmt.Sprintf("%s, %s.", subject, when)
	}
	return fmt.Sprintf(said.Say(headerWords.With), subject, in.Company, when)
}

// projectGoalLine states where the engagement stands and what it is running
// at. The target date is named only when the record carries one — a deadline
// nobody set is the invented context the grounding rule forbids.
func (said briefCopy) projectGoalLine(project ProjectIn) string {
	if project.TargetEndDate == nil {
		return fmt.Sprintf(said.Say(headerWords.Move), project.Name, said.phaseOrUnnamed(project.Phase))
	}
	return fmt.Sprintf(said.Say(headerWords.MoveTarget),
		project.Name, said.phaseOrUnnamed(project.Phase),
		project.TargetEndDate.Format(time.DateOnly))
}

func (said briefCopy) phaseOrUnnamed(phase string) string {
	if phase == "" {
		return said.Say(headerWords.Phase)
	}
	return phase
}

// priorMeetingLine says when this room last met and what it was called.
//
// Days ago, not a date, for the same reason the last-touch line is: the reader
// is placing it against today, and a date makes them do the arithmetic. The
// subject rides along because "three weeks ago" alone does not tell them which
// conversation they are being reminded of.
func (said briefCopy) priorMeetingLine(prior PriorMeetingIn, now time.Time, loc *time.Location) string {
	subject := prior.Subject
	if subject == "" {
		subject = said.Say(touchWords.Meeting)
	}
	days := elapsed.Days(prior.StartsAt, now, loc)
	switch {
	case days <= 0:
		return fmt.Sprintf(said.Say(touchWords.MetToday), subject)
	case days == 1:
		return fmt.Sprintf(said.Say(touchWords.MetYesterday), subject)
	default:
		return fmt.Sprintf(said.Say(touchWords.MetDays), days, subject)
	}
}
