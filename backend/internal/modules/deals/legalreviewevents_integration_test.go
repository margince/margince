// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// What a webhook subscriber is handed about a legal review: which deal, which
// review and what happened, and never the words either side wrote.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	secretRequestNote = "Our customer Sara Subject threatens to leave over clause nine"
	secretComments    = "Sara Subject's liability cap needs the board"
)

// reviewEnvelopes reads the outbox envelopes of one event type, oldest first.
func reviewEnvelopes(t *testing.T, e *reviewEnv, eventType string) []events.Envelope {
	t.Helper()
	rows, err := e.owner.Query(t.Context(),
		`SELECT envelope FROM event_outbox WHERE envelope->>'type' = $1 ORDER BY id`, eventType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.Envelope
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var env events.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		out = append(out, env)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// payloadKeys lists the fields of a payload, failing when it carries any words.
func payloadKeys(t *testing.T, env events.Envelope) map[string]any {
	t.Helper()
	whole, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{secretRequestNote, secretComments, "Sara Subject"} {
		if strings.Contains(string(whole), secret) {
			t.Fatalf("%s carries %q: %s", env.Type, secret, whole)
		}
	}
	var fields map[string]any
	if err := json.Unmarshal(env.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func wantFields(t *testing.T, got map[string]any, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("payload fields = %v, want exactly %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("payload %s = %v, want %s", key, got[key], value)
		}
	}
}

func TestALegalReviewIsPublishedAsTheFactAloneEvenWhenItHasWords(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	note := secretRequestNote
	requested, err := e.store.RequestLegalReview(e.asRequester(), dealID, RequestLegalReviewInput{Note: &note})
	if err != nil {
		t.Fatal(err)
	}
	review := ids.UUID(requested.Id)
	comments := secretComments
	if _, err := e.store.DecideLegalReview(e.asReviewer(decide), review, DecideLegalReviewInput{
		Outcome: ReviewChangesNeeded, Comments: &comments,
	}); err != nil {
		t.Fatal(err)
	}
	second := e.request(t, dealID, &review)
	if _, err := e.store.WithdrawLegalReview(e.asRequester(), second); err != nil {
		t.Fatal(err)
	}

	byReview := func(eventType string, id ids.UUID) events.Envelope {
		t.Helper()
		for _, env := range reviewEnvelopes(t, e, eventType) {
			if env.Entity.ID == id {
				return env
			}
		}
		t.Fatalf("no %s event for review %s", eventType, id)
		return events.Envelope{}
	}
	deal := dealID.String()
	asked := byReview("legal_review.requested", review)
	if asked.Entity.Type != "legal_review" {
		t.Errorf("entity type = %q, want legal_review", asked.Entity.Type)
	}
	wantFields(t, payloadKeys(t, asked), map[string]string{"deal_id": deal, "review_id": review.String()})
	wantFields(t, payloadKeys(t, byReview("legal_review.decided", review)),
		map[string]string{"deal_id": deal, "review_id": review.String(), "outcome": "changes_needed"})
	wantFields(t, payloadKeys(t, byReview("legal_review.withdrawn", second)),
		map[string]string{"deal_id": deal, "review_id": second.String()})
}
