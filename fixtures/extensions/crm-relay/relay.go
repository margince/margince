// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package relay is a test fixture: CI stages it, and a development build turns
// it on. It declares what no shipped unit uses. That is a tool, a per-member
// channel, a user secret, a job that postpones itself, an ingress source and
// SQL on its own tables. CI walks each from declaration to the
// running process. Its post tool lands an email-shaped message through capture by hand.
package relay

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

//go:embed migrations
var migrations embed.FS

// pollRetryDelay is how long an idle tick postpones itself: its own cadence.
const pollRetryDelay = 60 * time.Second

var classIdle = extension.FailureClass{
	Class:    "nothing_to_relay",
	Sentence: "the relay fixture found no note to relay",
	Remedy:   "nothing to do; the tick runs again at its next cadence",
}

// New returns the fixture's declaration.
func New() extension.Extension {
	return extension.Extension{
		Name:        "crm-relay",
		Version:     "0.1.0",
		Description: "A test fixture that walks a unit tool, channel, secret, job and table.",
		// A fixture note names no core record and holds no personal data.
		Privacy: []privacy.Table{
			{Name: "ext_crm_relay_note", Retention: jurisdiction.CommercialCorrespondence},
			{
				Name:      "ext_crm_relay_message",
				Links:     []privacy.Link{{Column: "activity_id", Record: privacy.Activity}},
				Personal:  []string{"document"},
				Retention: jurisdiction.CommercialCorrespondence,
				AgeColumn: "posted_at",
			},
		},
		Tools: []extension.Tool{
			{Name: "crm_relay_count_notes", Handle: countNotes},
			{Name: "crm_relay_post_message", Handle: postMessage},
		},
		Channels:       []extension.Channel{{Provider: "crm_relay", CredentialModel: extension.CredentialPerMember, Send: send, Live: live}},
		Secrets:        []extension.SecretsRequest{{Key: "token", Scope: extension.SecretScopeUser}},
		Jobs:           []extension.Job{{Name: "tick", Handle: tick}},
		FailureClasses: []extension.FailureClass{classIdle},
		Migrations:     migrations,
		Ingress: []extension.IngressSource{{
			System: "crm-relay",
			Lands:  []extension.RecordKind{extension.KindActivity},
			Merges: []extension.MergeKey{extension.MergeKeyEmail},
		}},
	}
}

func countNotes(ctx context.Context, rt extension.Runtime, _ json.RawMessage) (json.RawMessage, error) {
	n, err := notes(ctx, rt)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]int64{"notes": n})
}

// tick lands the posted test messages, and postpones itself by its cadence when
// there is nothing to land or relay.
func tick(ctx context.Context, rt extension.Runtime) error {
	landed, err := landPosted(ctx, rt)
	if err != nil {
		return err
	}
	n, err := notes(ctx, rt)
	if err != nil {
		return err
	}
	if landed == 0 && n == 0 {
		return extension.Reschedule(classIdle, pollRetryDelay, errors.New("crm-relay: no note to relay"))
	}
	return nil
}

func notes(ctx context.Context, rt extension.Runtime) (int64, error) {
	var n int64
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM ext.ext_crm_relay_note`).Scan(&n)
	})
	return n, err
}

func send(context.Context, extension.Runtime, extension.OutboundMessage) (extension.Receipt, error) {
	return extension.Receipt{}, nil
}

func live(context.Context, extension.Runtime, extension.UserID) (bool, error) {
	return true, nil
}
