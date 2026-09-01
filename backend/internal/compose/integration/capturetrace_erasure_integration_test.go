// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Art. 17 reaching the 24-hour capture trace, through the real eraser.
//
// The trace's sweep bounds exposure to a day; it does not ANSWER a request made
// inside that day, and an erasure honoured everywhere except one diagnostic
// table is not honoured. Driven through privacy.Eraser rather than by running
// its DELETE here: a test that pastes the production statement stays green if
// the cascade stops calling it.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

func TestErasureReachesTheCaptureTracePayloads(t *testing.T) {
	e := Setup(t)
	personID := seedSubject(t, e)

	// Two traced messages under the operator's payload posture: one from the
	// subject, one from somebody else. A purge that took both would be as wrong
	// as one that took neither.
	e.WsExec(t, `
		INSERT INTO capture_trace (user_id, connector, source_system, source_id,
		                           stage, outcome, counterparty, subject)
		VALUES (NULL, 'gmail', 'gmail', 'erasure-subject', 'tier_ladder', 'captured', $1, 'Quarterly numbers'),
		       (NULL, 'gmail', 'gmail', 'erasure-control', 'tier_ladder', 'captured', 'someone@else.test', 'Unrelated')`,
		subjectEmail)

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), personID, "test"); err != nil {
		t.Fatal(err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM capture_trace WHERE source_id = 'erasure-subject'`); n != 0 {
		t.Errorf("the erased subject's trace row survived: %d rows remain", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM capture_trace WHERE source_id = 'erasure-control'`); n != 1 {
		t.Errorf("another sender's trace row = %d, want 1 kept", n)
	}
}

// The OTHER lane, and the one the email test above cannot reach.
//
// A counterparty the pipeline knew by a provider ACCOUNT — a Telegram sender,
// an Official Account — is named in the trace by their DISPLAY NAME, because
// there is no address to write (capture/trace.go, traceChannelPayload). The
// email lane matches `counterparty` against an address, so every one of those
// rows survived an erasure that reported success.
//
// It was invisible while capture.trace_payloads defaulted off: the column it
// leaves behind was never written. Turning that default on is what made a
// dormant gap a live one, which is why the test arrives with the flip.
func TestErasureReachesAChannelCounterpartysTracePayloads(t *testing.T) {
	e := Setup(t)
	personID := seedSubject(t, e)

	// The subject's Telegram account, which is what makes this an identity the
	// erasure walks. Seeded through the same table the eraser reads.
	e.WsExec(t, `
		INSERT INTO person_channel_identity (person_id, provider, channel_user_id, source, captured_by)
		VALUES ($1, 'telegram', '99001', 'manual', 'human:x')`, personID)

	// Three rows. The subject's, by name on the provider they are known on; a
	// row naming somebody else on that provider; and the SAME name on a
	// different transport, which belongs to a different person and must stay.
	e.WsExec(t, `
		INSERT INTO capture_trace (user_id, connector, source_system, source_id,
		                           stage, outcome, counterparty, subject)
		VALUES (NULL, 'telegram', 'telegram', 'chan-subject', 'tier_ladder', 'captured', 'Selma Subject', 'Ping'),
		       (NULL, 'telegram', 'telegram', 'chan-control', 'tier_ladder', 'captured', 'Other Person', 'Unrelated'),
		       (NULL, 'zalo', 'zalo', 'chan-other-transport', 'tier_ladder', 'captured', 'Selma Subject', 'Unrelated')`)

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), personID, "test"); err != nil {
		t.Fatal(err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM capture_trace WHERE source_id = 'chan-subject'`); n != 0 {
		t.Errorf("the erased subject's channel trace row survived: %d rows remain", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM capture_trace WHERE source_id = 'chan-control'`); n != 1 {
		t.Errorf("another sender's channel trace row = %d, want 1 kept", n)
	}
	// The provider scope, which is the whole reason the DELETE names
	// source_system: a same-named person on a transport this subject was never
	// on is not this subject.
	if n := e.WsCount(t, `SELECT count(*) FROM capture_trace WHERE source_id = 'chan-other-transport'`); n != 1 {
		t.Errorf("a same-named counterparty on another transport = %d, want 1 kept", n)
	}
}
