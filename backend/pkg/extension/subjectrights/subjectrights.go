// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package subjectrights carries the data-subject-request contract of the
// published extension surface. A pack states how long its jurisdiction gives a
// controller to answer each kind of request. The core stamps the deadline.
//
// Like messaging rules, a rule set is data the core reads. A pack never
// computes a deadline itself. So one function turns a period into a date, and
// every jurisdiction gets the same month-end handling.
//
// These types are frozen published API from their first external consumer;
// they evolve additively or through versioned successors, never in place
// (EXT-P3).
//
//margince:extension-surface
package subjectrights

import (
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// RequestKind is the right a subject exercises. Closed, and the same set the
// request queue stores, so a pack cannot declare a deadline for a kind no
// request can carry.
type RequestKind string

const (
	// Access is the right to a copy of one's data (GDPR Art. 15).
	Access RequestKind = "access"
	// Rectify is the right to have it corrected (GDPR Art. 16).
	Rectify RequestKind = "rectify"
	// Erasure is the right to have it erased (GDPR Art. 17).
	Erasure RequestKind = "erasure"
)

// Validate enforces membership in the closed set.
func (k RequestKind) Validate() error {
	switch k {
	case Access, Rectify, Erasure:
		return nil
	}
	return fmt.Errorf("request kind %q is not one the request queue carries", string(k))
}

// Deadline is how long a controller has to answer, counted from receipt. It
// holds one of a calendar period and a number of hours, never both. Some laws
// count months and some count hours, and neither converts to the other.
type Deadline struct {
	Period jurisdiction.Period
	Hours  int
}

// maxHours bounds an hours deadline at a year, far below what time.Duration
// can hold, so Due cannot overflow.
const maxHours = 24 * 366

// Validate refuses a deadline that holds neither unit or both.
func (d Deadline) Validate() error {
	hasPeriod := d.Period != (jurisdiction.Period{})
	switch {
	case d.Hours < 0:
		return fmt.Errorf("a deadline of %d hours reaches back in time", d.Hours)
	case d.Hours > maxHours:
		return fmt.Errorf("a deadline of %d hours is longer than a year; state it as a calendar period", d.Hours)
	case hasPeriod && d.Hours > 0:
		return errors.New("a deadline names both a calendar period and hours; a law counts in one of them")
	case !hasPeriod && d.Hours == 0:
		return errors.New("a deadline names no period and no hours, so every request would be late on arrival")
	case hasPeriod:
		return d.Period.Validate()
	}
	return nil
}

// Due is the instant a request received at received must be answered by.
//
// Months add with the day CLAMPED to the target month's last day: one month
// after 31 January is 28 February, never 3 March as time.AddDate would say.
// Days and hours then add as they are.
func (d Deadline) Due(received time.Time) time.Time {
	if d.Hours > 0 {
		return received.Add(time.Duration(d.Hours) * time.Hour)
	}
	p := d.Period
	idx := received.Year()*12 + int(received.Month()) - 1 + p.Years*12 + p.Months
	year, month := idx/12, time.Month(idx%12+1)
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, received.Location()).Day()
	day := min(received.Day(), lastDay)
	clamped := time.Date(year, month, day,
		received.Hour(), received.Minute(), received.Second(), received.Nanosecond(), received.Location())
	return clamped.AddDate(0, 0, p.Days)
}

// Rules is one jurisdiction's request deadlines.
//
// A kind the map leaves out falls back to the core's GDPR month. So a pack
// states only the kinds its law sets differently.
type Rules struct {
	// Jurisdiction is whose deadlines these are, lower-case ISO 3166-1 alpha-2.
	Jurisdiction jurisdiction.Code

	// Version is the rule set's own version. A request records which version
	// set its deadline, so a later change to the law does not rewrite how an
	// old deadline reads.
	Version int

	// Instruments are the laws the deadlines rest on. The messaging contract's
	// type, so a citation is spelled the same way on both seams.
	Instruments []messaging.Instrument

	// Deadlines is the deadline per request kind.
	Deadlines map[RequestKind]Deadline
}

// Validate checks a whole rule set. The boot refuses a set it cannot apply,
// and a set that declares a deadline without naming the law it comes from.
func (r Rules) Validate() error {
	if err := r.Jurisdiction.Validate(); err != nil {
		return err
	}
	if r.Version <= 0 {
		return fmt.Errorf("subject-rights rules for %q carry version %d; a request records the version its deadline came from, and zero names nothing",
			string(r.Jurisdiction), r.Version)
	}
	if len(r.Deadlines) > 0 && len(r.Instruments) == 0 {
		return fmt.Errorf("subject-rights rules for %q declare deadlines and cite no instrument; a statutory deadline needs its source",
			string(r.Jurisdiction))
	}
	for _, in := range r.Instruments {
		if err := in.Validate(); err != nil {
			return fmt.Errorf("subject-rights rules for %q: %w", string(r.Jurisdiction), err)
		}
	}
	for kind, deadline := range r.Deadlines {
		if err := kind.Validate(); err != nil {
			return fmt.Errorf("subject-rights rules for %q: %w", string(r.Jurisdiction), err)
		}
		if err := deadline.Validate(); err != nil {
			return fmt.Errorf("subject-rights rules for %q, kind %q: %w", string(r.Jurisdiction), string(kind), err)
		}
	}
	return nil
}
