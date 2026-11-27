// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// A recipient with a live marketing grant, in a country whose pack fixes a
// subject label: everything but the label lets the message go.
func labelledRecipient(t *testing.T) (*capEnv, *Gate) {
	t.Helper()
	e := setupCap(t)
	e.seedMarketingSubject(t)
	code := testMessagingRules(t, messagingrules.Rules{Version: 1, SubjectPrefix: "(광고)"})
	e.subjectLivesIn(t, string(code))
	return e, NewGate(e.store)
}

func (e *capEnv) advertising(subject string) commsauthz.Request {
	return commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}}, LegacyPurposeKey: marketingPurposeKey,
		Context: commsauthz.CategoryMarketing, Subject: subject, Body: "Half price this week.",
	}
}

// errRolledBack undoes each staging so the next one starts from the same record.
var errRolledBack = errors.New("rolled back")

func TestStagingRefusesAnAdvertisingSubjectWithoutItsLabel(t *testing.T) {
	t.Parallel()
	e, gate := labelledRecipient(t)
	for subject, want := range map[string]string{
		"Our offer":      commsauthz.ReasonSubjectLabelMissing,
		"(광고) Our offer": "",
	} {
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			set, err := gate.AuthorizeStagingTx(e.ctx, tx, e.decisionDelivery, e.advertising(subject))
			if err != nil {
				return err
			}
			if d := set.Decisions[0]; (d.ReasonCode == commsauthz.ReasonSubjectLabelMissing) != (want != "") ||
				(want != "" && d.Verdict != commsauthz.VerdictDeny) {
				t.Errorf("%q staged as %s %q, want the label refusal = %v", subject, d.Verdict, d.ReasonCode, want != "")
			}
			return errRolledBack
		}); !errors.Is(err, errRolledBack) {
			t.Fatalf("staging %q: %v", subject, err)
		}
	}
}

func TestThePreviewRefusesAnAdvertisingSubjectWithoutItsLabel(t *testing.T) {
	t.Parallel()
	e, gate := labelledRecipient(t)
	set, err := gate.Preview(e.ctx, e.advertising("Our offer"))
	if err != nil {
		t.Fatal(err)
	}
	if d := set.Decisions[0]; d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonSubjectLabelMissing {
		t.Fatalf("preview %s %q, want the label refusal staging gives", d.Verdict, d.ReasonCode)
	}
}
