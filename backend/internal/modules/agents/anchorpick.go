// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Asking which record a name meant, in the chat, when the host can show a
// form. Each choice is a record the caller can read, named as read_record would
// name it, so the form shows nothing a read would not. Without a form the
// refusal names the candidates by id, as before.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const anchorPickKey = "pick_record"

// pickChoice is one record the form offers, as a titled oneOf entry.
type pickChoice struct {
	ID    string `json:"const"`
	Title string `json:"title"`
}

// pickAnchor asks the user which of hits they meant. It answers the zero id
// when it cannot ask, and the caller then refuses with the candidates.
func pickAnchor(
	ctx context.Context, records datasource.SystemOfRecordProvider, args anchorArgs, hits []RecordSearchHit,
) (ids.UUID, error) {
	if records == nil || !canAskInChat(ctx) {
		return ids.UUID{}, nil
	}
	choices := make([]pickChoice, 0, len(hits))
	offered := map[string]ids.UUID{}
	for _, hit := range hits {
		record, ok, err := readServable(ctx, records, hit.Ref)
		if err != nil {
			return ids.UUID{}, err
		}
		if ok {
			offered[hit.Ref.ID.String()] = hit.Ref.ID
			choices = append(choices, pickChoice{ID: hit.Ref.ID.String(), Title: choiceTitle(record, hit.Match)})
		}
	}
	if len(choices) < 2 {
		// One readable record is the one meant; none leaves the refusal.
		for _, only := range offered {
			return only, nil
		}
		return ids.UUID{}, nil
	}
	offer, err := json.Marshal(choices)
	if err != nil {
		return ids.UUID{}, err
	}
	question := chatQuestion{
		Key:     anchorPickKey,
		Message: "Which " + args.RecordType + " did you mean by \"" + args.RecordName + "\"?",
		Schema: json.RawMessage(`{"type":"object","required":["record_id"],
			"properties":{"record_id":{"type":"string","oneOf":` + string(offer) + `}}}`),
	}
	reply, err := replyInChat(ctx, question)
	if err != nil {
		return ids.UUID{}, err
	}
	if !reply.Accepted {
		return ids.UUID{}, errDeclinedInChat
	}
	var picked string
	if json.Unmarshal(reply.Content["record_id"], &picked) == nil {
		if id, ok := offered[picked]; ok {
			return id, nil
		}
	}
	// An answer that is none of the choices is asked again, as the
	// specification asks of a retry that lacks what the call needs.
	return ids.UUID{}, &askInChatError{question: question}
}

// choiceTitle names a record as read_record would, with the excerpt the
// search showed beside it when there is one.
func choiceTitle(record datasource.Record, match RecordMatch) string {
	title := strings.Trim(recordLabel(record), `"`)
	switch {
	case match.WorksAt != nil && match.WorksAt.CompanyName != "":
		return title + " (" + match.WorksAt.CompanyName + ")"
	case match.Snippet != "":
		return title + " (" + match.Snippet + ")"
	default:
		return title
	}
}
