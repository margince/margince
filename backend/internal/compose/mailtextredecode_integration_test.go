// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail stored before the charset fix, over a real Postgres.
//
// The repair runs through the real executor. What is worth proving is that the
// step is wired into the task and settles, not that a function decodes a
// string.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// windows1252Original is a message whose body declares the charset its bytes
// are in, with a subject left as an encoded word.
func windows1252Original(sourceID string) []byte {
	body := []byte{'G', 'r', 0xFC, 0xDF, 'e', '.'}
	head := strings.Join([]string{
		"From: anna@customer.example",
		"To: us@margince.test",
		"Subject: =?Windows-1252?Q?Gr=FC=DFe?=",
		"Date: " + time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC).Format(time.RFC1123Z),
		"Message-ID: <" + sourceID + ">",
		"Content-Type: text/plain; charset=Windows-1252",
		"", "",
	}, "\r\n")
	return append([]byte(head), body...)
}

// seedMisdecodedMail stores the pair a parser that ignored the charset would
// have written, beside the original it was written from.
func seedMisdecodedMail(t *testing.T, e *integration.Env, sourceID, subject, body string) ids.UUID {
	t.Helper()
	// Encoded the way capture stores an original, because the bytes are not
	// UTF-8 and the repair reads them back through capture's own decoder.
	payload, err := capture.EncodeStoredOriginal(windows1252Original(sourceID))
	if err != nil {
		t.Fatalf("encoding the stored original: %v", err)
	}
	var id ids.UUID
	// raw_capture_id is set, because the join reads the link and not the
	// Message-ID: an original found by a key a sender typed may be planted.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			WITH rc AS (INSERT INTO raw_capture (source_system, source_id, payload)
			            VALUES ('email', $1, $4::jsonb) RETURNING id)
			INSERT INTO activity (kind, subject, body, direction, occurred_at, source_system, source_id, source,
			                      captured_by, body_completeness, raw_capture_id)
			SELECT 'email', $2, $3, 'inbound', now(), 'email', $1, 'gmail:seed',
			       'connector:gmail', 'complete', rc.id FROM rc RETURNING id`,
			sourceID, subject, body, string(payload)).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding mis-decoded mail: %v", err)
	}
	return id
}

func runMailTextRedecode(ctx context.Context, t *testing.T, e *integration.Env) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{mailBodyCompletenessDataTask()}, "test",
		func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func mailTextOf(t *testing.T, e *integration.Env, id ids.UUID) (subject, body string) {
	t.Helper()
	subject = e.WsScalar(t, `SELECT coalesce(subject, '') FROM activity WHERE id = $1`, id)
	body = e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, id)
	return subject, body
}

// The stored prefix capture writes above a mail body. A fixture that leaves it
// out gives a body of a different length from the re-parse, which the repair
// declines. The case would then pass over the repair rather than through it.
const storedMailPrefix = "From: anna@customer.example\nTo: us@margince.test\n\n"

func TestMailStoredWithoutItsCharsetReadsAgain(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	id := seedMisdecodedMail(t, e, "redecode-one@customer.example",
		"=?Windows-1252?Q?Gr=FC=DFe?=", storedMailPrefix+"Gr??e.")

	runMailTextRedecode(ctx, t, e)

	subject, body := mailTextOf(t, e, id)
	if subject != "Grüße" {
		t.Errorf("subject reads %q, want the decoded one", subject)
	}
	if !strings.Contains(body, "Grüße.") {
		t.Errorf("body reads %q, want the umlauts back", body)
	}
}

// A replay settles: the second pass finds text that already reads and leaves it.
func TestAReplayOfTheRedecodeChangesNothing(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	id := seedMisdecodedMail(t, e, "redecode-twice@customer.example",
		"=?Windows-1252?Q?Gr=FC=DFe?=", storedMailPrefix+"Gr??e.")

	runMailTextRedecode(ctx, t, e)
	first, firstBody := mailTextOf(t, e, id)
	runMailTextRedecode(ctx, t, e)
	second, secondBody := mailTextOf(t, e, id)

	if first != second || firstBody != secondBody {
		t.Errorf("a replay rewrote the text: %q/%q then %q/%q", first, firstBody, second, secondBody)
	}
}

// Text that already reads is not rewritten, so mail captured today is untouched
// by a pass over it.
func TestMailThatAlreadyReadsIsLeftAlone(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	id := seedMisdecodedMail(t, e, "redecode-none@customer.example",
		"Grüße", storedMailPrefix+"Grüße.")

	runMailTextRedecode(ctx, t, e)

	subject, body := mailTextOf(t, e, id)
	if subject != "Grüße" || !strings.Contains(body, "Grüße.") {
		t.Errorf("a readable pair was changed: %q / %q", subject, body)
	}
}
