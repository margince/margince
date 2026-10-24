// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// TurnTextRunes bounds the text of every turn a draft reads, whoever wrote it.
// One bound for all of them keeps a follow-up from reading our own earlier
// mail at half the length it reads theirs.
const TurnTextRunes = 800

// Turn is one exchange of the conversation as a drafting prompt reads it.
type Turn struct {
	ID        string
	Kind      crmcontracts.ActivityKind
	Direction string
	At        time.Time
	Subject   string
	// Text is the authored opening of the message or the logged note, empty
	// for a task or a call, which carries a subject and nothing more.
	Text string
	// Logged marks a note or meeting a rep wrote down. It is context for the
	// draft, never the message being continued.
	Logged bool
	// AfterAnchor is true when the turn is newer than the selected message.
	AfterAnchor bool
}

// TurnText is the opening of a stored body, bounded the way every turn is.
func TurnText(body string) string {
	return textlang.MessageOpening(body, TurnTextRunes)
}

// Conversation folds newest-first activities into at most limit turns. It is
// the one place that decides which rows a draft reads and how much of each:
// the reply drafter hands it a thread, and the contact and lead drafters hand
// it the record's recent activity.
//
// A row is left out when its content is withheld from this reader, when it is
// the anchor itself, or when it is a note or meeting that records no contact
// that happened (see IsLoggedTouch). Our own sent mail carries its text like
// theirs, because a follow-up has to know what we already asked.
func Conversation(activities []crmcontracts.Activity, anchor *crmcontracts.Activity, now time.Time, limit int) []Turn {
	var turns []Turn
	for _, activity := range activities {
		if len(turns) == limit {
			break
		}
		if anchor != nil && activity.Id == anchor.Id {
			continue
		}
		if activity.ContentState != nil && *activity.ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		logged := IsLoggedKind(activity.Kind)
		if logged && !IsLoggedTouch(activity, now) {
			continue
		}
		turn := Turn{
			ID:     activity.Id.String(),
			Kind:   activity.Kind,
			At:     activity.OccurredAt,
			Logged: logged,
		}
		if activity.Direction != nil {
			turn.Direction = string(*activity.Direction)
		}
		if activity.Subject != nil {
			turn.Subject = *activity.Subject
		}
		if activity.Body != nil && (logged || activity.Kind == crmcontracts.ActivityKindEmail) {
			turn.Text = TurnText(*activity.Body)
		}
		turn.AfterAnchor = anchor != nil && activity.OccurredAt.After(anchor.OccurredAt)
		turns = append(turns, turn)
	}
	return turns
}
