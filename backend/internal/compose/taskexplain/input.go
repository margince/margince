// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// floorVersion is bumped by hand when the floor's wording changes, because
// its sentences are Go formats with nothing to hash.
const floorVersion = "task-explanation-floor-v1"

// promptVersion is derived from the prompt as sent, so a wording edit
// rewrites every cached explanation.
var promptVersion = ai.PromptDigest(func(fence promptfence.Fence) string {
	return systemFor(fence, string(textlang.English))
})

// sourceExcerpt bounds the source message's text in the prompt. A promise or
// an ask sits near the top of the message that made it.
const sourceExcerpt = 1200

// Input is what one explanation is written from.
type Input struct {
	Task   TaskIn              `json:"task"`
	Source *contactbrief.ActIn `json:"source_message,omitempty"`
	// SourceText is the source message's own words, cut to sourceExcerpt.
	SourceText   string              `json:"source_text,omitempty"`
	Relationship *contactbrief.Input `json:"relationship,omitempty"`
}

// TaskIn is the task as its reader sees it.
type TaskIn struct {
	ID    string `json:"id"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title"`
	Text  string `json:"text,omitempty"`
	// Origin is what filed the task: a promise read from mail, a customer's
	// request, an automation rule, or a person.
	Origin  string `json:"origin"`
	Created string `json:"created"`
	Due     string `json:"due,omitempty"`
	Overdue bool   `json:"overdue,omitempty"`
}

// FromTask folds the task the caller read.
func FromTask(task crmcontracts.Activity, now time.Time) TaskIn {
	in := TaskIn{
		ID:      task.Id.String(),
		Origin:  task.Source,
		Created: task.CreatedAt.UTC().Format(time.DateOnly),
	}
	if task.TaskType != nil {
		in.Type = string(*task.TaskType)
	}
	in.Title, in.Text = readerWords(task)
	if task.DueAt != nil {
		in.Due = task.DueAt.UTC().Format(time.DateOnly)
		in.Overdue = task.DueAt.Before(now)
	}
	return in
}

// readerWords is the task's title and text as its reader sees them:
// Margince's own words in the reader's language, else what the author wrote.
func readerWords(task crmcontracts.Activity) (string, string) {
	title, text := deref(task.Subject), deref(task.Body)
	if display := task.SystemDisplay; display != nil {
		if display.Subject != nil {
			title = *display.Subject
		}
		if display.Body != nil {
			text = *display.Body
		}
	}
	return title, text
}

// FromSource folds the message the task came from. A message whose content
// the reader may not read keeps only its kind and date.
func FromSource(source crmcontracts.Activity) (*contactbrief.ActIn, string) {
	folded := &contactbrief.ActIn{
		ID:      source.Id.String(),
		Kind:    string(source.Kind),
		At:      activitywhen.Text(source),
		Speaker: contactbrief.SpeakerFor(source.Voice),
	}
	if source.ContentState != nil && *source.ContentState == crmcontracts.ActivityContentStateWithheld {
		folded.Withheld = true
		folded.Speaker = nil
		return folded, ""
	}
	folded.Subject = deref(source.Subject)
	if source.EmailSummary != nil && source.EmailSummary.Preview != nil {
		folded.Preview = *source.EmailSummary.Preview
	}
	return folded, mailcopy.Snippet(deref(source.Body), sourceExcerpt)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// knownRecords is every record a sentence may cite, with what the model was
// shown of it: the task, its source and the relationship's own records.
func knownRecords(in Input) map[claims.Evidence]string {
	known := map[claims.Evidence]string{}
	if in.Relationship != nil {
		known = contactbrief.KnownRecords(*in.Relationship)
	}
	task := claims.Evidence{EntityType: citeActivity, EntityID: in.Task.ID}
	known[task] += claims.Source(in.Task)
	if in.Source != nil {
		source := claims.Evidence{EntityType: citeActivity, EntityID: in.Source.ID}
		known[source] += claims.Source(in.Source) + "\n" + in.SourceText
	}
	return known
}

// Fingerprint names the input, prompt, floor, routing and language an
// explanation was written under.
func Fingerprint(in Input, routingVersion, lang string) (string, error) {
	encoded, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("fingerprint the task explanation input: %w", err)
	}
	sum := sha256.Sum256([]byte(floorVersion + "\x00" + promptVersion + "\x00" +
		routingVersion + "\x00" + lang + "\x00" + string(encoded)))
	return hex.EncodeToString(sum[:]), nil
}
