// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsummary

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/approvals"
)

// Summary budgets bound input and output while quoted evidence stays grounded in its source.
const (
	maxMessages       = 1000
	maxInputRunes     = 120000
	BriefSummaryRunes = 1200
)

// ErrIncompleteInput prevents a partial source from being presented as a complete recap.
var ErrIncompleteInput = errors.New("chat summary requires the complete source within its input budget")

func completeInput(in Input) error {
	if len(in.Messages) > maxMessages {
		return ErrIncompleteInput
	}
	total := utf8.RuneCountInString(in.Record) + utf8.RuneCountInString(in.Channel) + utf8.RuneCountInString(in.Earlier)
	for _, fact := range in.Facts {
		total += utf8.RuneCountInString(fact)
	}
	for _, message := range in.Messages {
		if message.Side < SideUnknown || int(message.Side) >= len(sideLabels) {
			return ErrIncompleteInput
		}
		total += utf8.RuneCountInString(message.Author) + utf8.RuneCountInString(message.Text)
		for _, file := range message.Files {
			total += utf8.RuneCountInString(file.Name) + utf8.RuneCountInString(file.Type)
		}
	}
	if total > maxInputRunes {
		return ErrIncompleteInput
	}
	return nil
}

// CheckForInput checks recap bounds and reconfirmation quotes against the supplied source.
func CheckForInput(reply Reply, in Input) error {
	if question := reply.Reconfirmation; question != nil {
		if !reply.Relevant || strings.TrimSpace(question.Question) == "" || strings.TrimSpace(question.Quote) == "" || utf8.RuneCountInString(question.Question) > maxItemRunes || utf8.RuneCountInString(question.Quote) > approvals.MaxEvidenceSnippet {
			return errors.New("a reconfirmation needs a bounded question and an exact source quote")
		}
		grounded := false
		for _, message := range in.Messages {
			if strings.Contains(message.Text, question.Quote) {
				grounded = true
				break
			}
		}
		if !grounded {
			return errors.New("the reconfirmation quote is absent from the allowed source")
		}
	}

	if err := Check(reply); err != nil {
		return err
	}
	if in.Brief && utf8.RuneCountInString(reply.Summary) > BriefSummaryRunes {
		return errors.New("the brief summary exceeds its length limit")
	}
	return nil
}
