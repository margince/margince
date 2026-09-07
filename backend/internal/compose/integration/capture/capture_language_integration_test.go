// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What language a captured message records.
//
// The column has existed since the baseline and nothing ever wrote it, so a
// drafted reply had to re-read the body every time — and a body grows a quoted
// chain in whatever languages the exchange has since used. These prove capture
// records what the message was written in when it arrived, through the real
// sink rather than an insert of our own.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACapturedMessageRecordsTheLanguageItIsWrittenIn(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	for _, tc := range []struct {
		name    string
		id      string
		subject string
		body    string
		want    string
	}{
		{
			name:    "an English message",
			id:      "lang-en",
			subject: "Outstanding invoice",
			body:    "Hello, I am writing about the invoice that has been outstanding since December.",
			want:    "en",
		},
		{
			name:    "a German message",
			id:      "lang-de",
			subject: "Offene Rechnung",
			body:    "Guten Tag, ich melde mich wegen der Rechnung, die seit Dezember offen ist.",
			want:    "de",
		},
		{
			// Vietnamese is why the constraint had to be widened: the product
			// ships it and the CHECK admitted only de and en.
			name:    "a Vietnamese message",
			id:      "lang-vi",
			subject: "Hóa đơn chưa thanh toán",
			body:    "Xin chào, tôi viết thư này về hóa đơn vẫn chưa được thanh toán từ tháng mười hai.",
			want:    "vi",
		},
		{
			// The subject carries the evidence the body does not. Read in that
			// order, so a reply whose subject is still in the sender's language
			// does not override a body that says otherwise.
			name:    "a body too short to tell, under a German subject",
			id:      "lang-subject",
			subject: "Guten Tag, wie besprochen sende ich Ihnen die Unterlagen",
			body:    "ok",
			want:    "de",
		},
		{
			// Unknown records NULL, which is what every row carried before and
			// what the search index already treats as "do not stem".
			name:    "a message with nothing to go on",
			id:      "lang-none",
			subject: "ok",
			body:    "ok",
			want:    "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sync(t, emailWith(tc.id+"@acme.example", tc.subject, tc.body))
			got := languageOf(t, e, newestActivity(t, e))
			if got != tc.want {
				t.Errorf("recorded language = %q, want %q", got, tc.want)
			}
		})
	}
}

// A replay must not rewrite what the first capture recorded: the row is stored
// once on its natural key, and the second sync writes nothing at all.
func TestAReplayDoesNotRewriteTheRecordedLanguage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	raw := emailWith("lang-replay@acme.example", "Outstanding invoice",
		"Hello, I am writing about the invoice that has been outstanding since December.")

	sync(t, raw)
	activityID := newestActivity(t, e)
	if got := languageOf(t, e, activityID); got != "en" {
		t.Fatalf("the fixture needs an English message, got %q", got)
	}
	sync(t, raw)
	if got := languageOf(t, e, activityID); got != "en" {
		t.Errorf("a replay changed the recorded language to %q", got)
	}
}

// emailWith is one inbound message with the subject and body a case is about.
//
// UTF-8 declared and 8bit-encoded rather than folded into ASCII: the
// Vietnamese case is decided by its diacritics, and a fixture that stripped
// them would test a message nobody ever sends.
func emailWith(msgID, subject, body string) []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>",
		"To: " + captureOwner,
		"Subject: " + subject,
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
		"",
	}, "\r\n"))
}

// The label is read from the text, so editing the text retires it.
//
// A stored language that outlived the words it described would send a reply in
// the language the message USED to be in — which is the same defect as the
// corpus deciding, arriving by a slower route.
func TestEditingAMessageRetiresItsRecordedLanguage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, emailWith("lang-edit@acme.example", "Outstanding invoice",
		"Hello, I am writing about the invoice that has been outstanding since December."))
	activityID := newestActivity(t, e)
	if got := languageOf(t, e, activityID); got != "en" {
		t.Fatalf("the fixture needs an English message, got %q", got)
	}

	store := activities.NewStore(e.DB())
	german := "Guten Tag, ich melde mich wegen der Rechnung, die seit Dezember offen ist."
	if _, err := store.UpdateActivity(writerCtx(e, e.Rep1),
		ids.From[ids.ActivityKind](activityID),
		activities.UpdateActivityInput{Body: &german}); err != nil {
		t.Fatalf("editing the message: %v", err)
	}
	if got := languageOf(t, e, activityID); got != "" {
		t.Errorf("the edited message still records %q, which describes text that is gone", got)
	}
}

// writerCtx is one seat editing correspondence they may write.
func writerCtx(e *integration.SearchEnv, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// languageOf reads what one activity records about its own language, with the
// empty string standing for the NULL an undetectable message carries.
func languageOf(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) string {
	t.Helper()
	var language *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT language FROM activity WHERE id = $1`, activityID).Scan(&language)
	})
	if err != nil {
		t.Fatalf("reading the recorded language: %v", err)
	}
	if language == nil {
		return ""
	}
	return *language
}

// newestActivity is the row the sync just wrote.
func newestActivity(t *testing.T, e *integration.SearchEnv) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE kind = 'email' ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id)
	})
	if err != nil {
		t.Fatalf("finding the captured message: %v", err)
	}
	return id
}
