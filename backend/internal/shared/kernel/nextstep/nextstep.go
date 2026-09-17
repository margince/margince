// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package nextstep answers one question for every surface that asks it: does
// this deal have a next step, or has nobody agreed one?
//
// It exists because two surfaces answered it separately and disagreed in front
// of a reader. The contact page said "No next step with them on an open deal —
// book a meeting"; the deal page, about the same deal on the same morning, said
// nothing needed doing. Both were reading real records. Neither was reading the
// same rule.
//
// WHAT IS SHARED IS THIS BOOLEAN, NOT ITS INPUTS. The two surfaces gather
// different facts on purpose, and a reader who expects them to agree about
// everything will be wrong about something real:
//
//	The deal card counts work linked to the DEAL. The contact page counts work
//	this CONTACT reaches — its own link, or their participation. A task filed
//	with another stakeholder settles the deal's rung and leaves this contact's
//	standing open, because a step agreed with somebody else is not a step
//	agreed with them.
//
// So the agreement this package buys is narrow and worth stating: given the
// same three facts, both surfaces reach the same verdict. Where their facts
// differ, their verdicts may differ, and that difference is a fact about the
// records rather than a bug.
//
// TWO INPUTS ARE DELIBERATELY ABSENT.
//
// An owed reply is not here. Each ladder ranks an unanswered message ABOVE this
// rung — the deal card's unansweredInbound, the contact page's re_engaged rung —
// so folding it in would push a page with a fresh unanswered message down to
// "nothing needs you", which is the opposite of true.
//
// A meeting's horizon is not here either. MeetingBooked means booked at all, at
// any distance. How soon a meeting has to be before preparing for it is the most
// useful thing a reader could do is each surface's own rung and its own number.
// A meeting booked in six weeks is still a next step; it is just not today's.
package nextstep

// Facts are what a surface has already read. Every field is a fact about
// records, never a judgement: the caller decides what counts as a human task
// and what counts as booked, and this decides only what those facts mean
// together.
type Facts struct {
	// DealOpen says a live deal is in play. A closed deal has no next step to
	// agree, which is why it is the first thing asked.
	DealOpen bool
	// MeetingBooked says something is on the calendar with the other side, at
	// any distance. See the package doc for why no horizon lives here.
	MeetingBooked bool
	// OpenHumanTasks counts work a colleague filed and has not finished. It must
	// EXCLUDE what the product minted for itself — a check-in reminder, a
	// forecast-assurance review — because the question is what somebody
	// agreed to do, and the product agreeing with itself is not an answer.
	//
	// The count must come from an authoritative read, not from filtering a
	// capped page: a page of system reminders can hide the one human task
	// behind it, and a caller that filtered the page would report a missing
	// next step that is not missing. principal.SystemMintedID and
	// activities.NotSystemMinted are the two spellings of the exclusion.
	OpenHumanTasks int
}

// Missing reports that a live deal has nothing agreed as its next step.
//
// True is the finding: a deal is in play, nothing is on the calendar, and
// nobody has written down what happens next. That is a gap somebody can close
// today, which is why both surfaces open on it.
func Missing(f Facts) bool {
	return f.DealOpen && !f.MeetingBooked && f.OpenHumanTasks == 0
}
