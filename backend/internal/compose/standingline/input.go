// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package standingline writes the one sentence How it stands shows about a
// contact, company or project: where things stand after its latest activity.
//
// It is written in the background when an activity lands, kept until the
// next one changes the picture, and shared by the record's readers. Its
// evidence is every activity it was written from, so a reader sees it only
// while they may open each of them.
package standingline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// RecentActivities is how many of the newest activities a line is written
// from. That is enough for the thread's last turn, and few enough for one
// reader to open them all.
const RecentActivities = 5

// excerptRunes bounds the body quoted for one activity.
const excerptRunes = 600

// The sides an activity's author can stand on.
const (
	sideCustomer = "customer"
	sideOurs     = "our team"
	sideSystem   = "system"
)

// Input is what a line is written from.
type Input struct {
	// Kind is the record's type: contact, company or project.
	Kind string `json:"record_kind"`
	// Previous is the line shown now, kept when nothing changed the picture.
	Previous string `json:"previous_line,omitempty"`
	// Recent is newest first.
	Recent []Act `json:"recent"`
}

// Act is one activity as the line may quote it.
type Act struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	At      string `json:"at"`
	Author  string `json:"author,omitempty"`
	Side    string `json:"side,omitempty"`
	Subject string `json:"subject,omitempty"`
	Text    string `json:"text,omitempty"`
	Status  string `json:"status,omitempty"`
}

// FromActivities folds the record's newest activities. A row whose content is
// withheld is left out. So a reader who may open the evidence could read all
// the text the line came from.
func FromActivities(kind, previous string, rows []crmcontracts.Activity) Input {
	in := Input{Kind: kind, Previous: previous}
	for _, row := range rows {
		if len(in.Recent) == RecentActivities {
			break
		}
		if row.ContentState != nil && *row.ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		act := Act{ID: row.Id.String(), Kind: string(row.Kind), At: activitywhen.Text(row), Subject: deref(row.Subject)}
		act.Author, act.Side = authorOf(row.Voice)
		act.Text = mailcopy.Snippet(deref(row.Body), excerptRunes)
		if summary := row.EmailSummary; summary != nil && summary.Preview != nil {
			act.Text = *summary.Preview
		}
		switch {
		case row.IsDone != nil && *row.IsDone:
			act.Status = "done"
		case row.MeetingStatus != nil:
			act.Status = string(*row.MeetingStatus)
		}
		in.Recent = append(in.Recent, act)
	}
	return in
}

// authorOf names who wrote an activity and which side they are on. The line is
// shared, so it names who wrote and never says "you".
func authorOf(voice *crmcontracts.ActivityVoice) (string, string) {
	if voice == nil || voice.Author == nil {
		return "", ""
	}
	author := voice.Author
	name := strings.TrimSpace(deref(author.DisplayName))
	if name == "" {
		name = strings.TrimSpace(deref(author.Address))
	}
	switch author.Kind {
	case crmcontracts.ActivityAuthorKindMachine:
		return name, sideSystem
	case crmcontracts.ActivityAuthorKindColleague:
		return name, sideOurs
	case crmcontracts.ActivityAuthorKindContactRecord:
		return name, sideCustomer
	default:
		return name, ""
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Activities lists the ids of the activities the line was written from.
func (in Input) Activities() []string {
	out := make([]string, len(in.Recent))
	for i, act := range in.Recent {
		out[i] = act.ID
	}
	return out
}

func knownRecords(in Input) map[claims.Evidence]string {
	known := make(map[claims.Evidence]string, len(in.Recent))
	for _, act := range in.Recent {
		known[claims.Evidence{EntityType: citeActivity, EntityID: act.ID}] = claims.Source(act)
	}
	return known
}

// promptVersion changes the fingerprint whenever the prompt does.
var promptVersion = ai.PromptDigest(func(fence promptfence.Fence) string {
	return systemFor(fence, string(textlang.English))
})

// Fingerprint names the activities, prompt, routing and language a line was
// written under. The previous line is left out: keeping it is the answer.
func Fingerprint(in Input, routingVersion, lang string) (string, error) {
	encoded, err := json.Marshal(in.Recent)
	if err != nil {
		return "", fmt.Errorf("fingerprint the status line input: %w", err)
	}
	sum := sha256.Sum256([]byte(promptVersion + "\x00" + routingVersion + "\x00" + lang + "\x00" +
		in.Kind + "\x00" + string(encoded)))
	return hex.EncodeToString(sum[:]), nil
}
