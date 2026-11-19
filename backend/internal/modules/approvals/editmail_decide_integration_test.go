// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A mail edit that blanks, drops or mistypes the subject or body is refused
// through Decide, and the row stays pending with its staged text.
func TestDecideRefusesAMailEditWithoutAWrittenSubjectAndBody(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asHumanWith(grantsFor(map[string]principal.ObjectGrant{
		objectActivity: {Create: true, Read: true},
		tableContact:   {Read: true},
	}))
	rest := func(body string) string {
		return `{"operation":"sendEmail","path":"/v1/activities/a1/send-email","body":` + body + `}`
	}
	staged := rest(`{"to":["anna@example.test"],"subject":"Hi","body":"Text"}`)
	cases := []struct{ name, edited string }{
		{"empty body object", rest(`{}`)},
		{"null and number", rest(`{"subject":null,"body":42}`)},
		{"blank subject", rest(`{"subject":" ","body":"Text"}`)},
		{"missing body", rest(`{"subject":"Hi"}`)},
	}
	for _, tc := range cases {
		id, err := e.svc.Stage(ctx, StageInput{
			Kind:           "send_email",
			ProposedChange: json.RawMessage(staged),
			DiffHash:       "hash-" + tc.name,
			Summary:        "Reply to Anna?",
		})
		if err != nil {
			t.Fatalf("%s: staging: %v", tc.name, err)
		}
		_, decideErr := e.svc.DecideEdited(ctx, id, json.RawMessage(tc.edited))
		if _, ok := errors.AsType[*InvalidEditError](decideErr); !ok {
			t.Errorf("%s: DecideEdited = %v, want *InvalidEditError", tc.name, decideErr)
		}
		after, err := e.svc.Get(ctx, id)
		if err != nil || after.Status != "pending" {
			t.Errorf("%s: row after the refusal = (%v, %v), want pending", tc.name, after.Status, err)
		}
	}
}
