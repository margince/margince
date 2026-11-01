// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDirectSenderCorrectionRestoresOnlyProviderProofFromStoredOriginals(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	for _, server := range []string{"mx.google.com", "untrusted.example"} {
		contact := seedDirectSenderDuty(t, e, "unknown_legacy", "open", "connector:gmail")
		address := contact.String() + "@example.test"
		source := contact.String() + "@proof.test"
		original := fmt.Sprintf("From: Dana <%s>\r\nTo: owner@demo.test\r\nMessage-ID: <%s>\r\nDate: Wed, 06 May 2026 10:00:00 +0000\r\nAuthentication-Results: %s; dmarc=pass header.from=example.test\r\nContent-Type: text/plain\r\n\r\nEvent sponsorship", address, source, server)
		e.WsExec(t, `WITH raw AS (INSERT INTO raw_capture (source_system, source_id, payload)
   VALUES ('email', @source, to_jsonb(@original::text)) RETURNING id)
   UPDATE activity a SET source_system = 'email', source_id = @source, raw_capture_id = raw.id,
   counterparty_email = @address, inbound_sender_authenticated = false FROM raw
   WHERE EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.address = @address)`, pgx.NamedArgs{"source": source, "original": original, "address": address})
	}
	task := subjectMailDataTask(func() time.Time { return now })
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	runMachineAddressTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'exempt_with_reason'`); got != 1 {
		t.Fatalf("settled %d cases, want only the authenticated original", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'open'`); got != 1 {
		t.Fatalf("unverified cases remaining = %d", got)
	}
}
