// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestNoticePresentationUsesTheReaderLanguageWithoutChangingItsIdentity(t *testing.T) {
	e := setupNotices(t)
	for _, base := range textlang.Shipped {
		subject := fmt.Sprintf(systemtext.Copy("lead.reply", base), "Example")
		body := systemtext.Copy("automation.new_lead", base)
		input := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: subject, Body: body, DedupeKey: "language:" + string(base), SystemText: &systemtext.Message{Subject: systemtext.Sentence("lead.reply", systemtext.Label("Example")), Body: systemtext.Sentence("automation.new_lead")}}
		id, err := e.store.Create(e.engineCtx(), input)
		if err != nil {
			t.Fatal(err)
		}
		for _, reader := range textlang.Shipped {
			rows, err := e.store.UnreadFor(systemtext.WithLocale(e.asUser(e.recipient), reader), 20)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range rows {
				if row.ID != id {
					continue
				}
				found = true
				if row.Subject != subject || row.Body != body || row.SystemDisplay == nil || row.SystemDisplay.Subject == nil || *row.SystemDisplay.Subject != fmt.Sprintf(systemtext.Copy("lead.reply", reader), "Example") {
					t.Fatalf("base=%s reader=%s notice=%+v", base, reader, row)
				}
				if shown := row.InLanguage(reader); shown.Subject != *row.SystemDisplay.Subject || shown.ID != id {
					t.Fatalf("delivery copy disagrees with the reader: %+v", shown)
				}
			}
			if !found {
				t.Fatal("notice disappeared in another language")
			}
		}
		input.Subject = "A different replay caption"
		input.SystemText = nil
		replay, err := e.store.Create(e.engineCtx(), input)
		if err != nil || replay != id {
			t.Fatalf("dedupe identity changed: %v %v", replay, err)
		}
	}
}

func TestNoticeLimitsKeepEveryLanguageAndEditsRetireMetadata(t *testing.T) {
	e := setupNotices(t)
	label := strings.Repeat("長", 180)
	subject := fmt.Sprintf(systemtext.Copy("lead.reply", textlang.English), label)
	input := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: subject, Body: systemtext.Literal("automation.new_lead"), SystemText: &systemtext.Message{Subject: systemtext.Sentence("lead.reply", systemtext.Label(label)), Body: systemtext.Sentence("automation.new_lead")}}
	id, err := e.store.Create(e.engineCtx(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range textlang.Shipped {
		rows, err := e.store.UnreadFor(systemtext.WithLocale(e.asUser(e.recipient), language), 20)
		if err != nil || len(rows) != 1 {
			t.Fatalf("notice read: %v %v", rows, err)
		}
		display := rows[0].SystemDisplay
		if display == nil || display.Subject == nil || utf8.RuneCountInString(*display.Subject) > subjectBound || !strings.Contains(*display.Subject, "…") {
			t.Fatalf("%s cap: %+v", language, display)
		}
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE notice SET subject=@subject WHERE id=@id`, pgx.StrictNamedArgs{"subject": "My own caption", "id": id}); err != nil {
		t.Fatal(err)
	}
	var message *systemtext.Message
	if err := e.owner.QueryRow(context.Background(), `SELECT system_text FROM notice WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if message == nil || message.Subject != nil || message.Body == nil {
		t.Fatalf("direct text edit retained stale metadata: %+v", message)
	}
}

func TestInvalidNoticeMetadataCannotPreventAValidNotice(t *testing.T) {
	e := setupNotices(t)
	_, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Caption", SystemText: &systemtext.Message{Subject: systemtext.Sentence("missing")}})
	if err != nil {
		t.Fatalf("display metadata blocked notice creation: %v", err)
	}
	var count int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM notice WHERE recipient_user_id=@recipient AND system_text IS NULL`, pgx.StrictNamedArgs{"recipient": e.recipient}).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notice was lost or retained invalid metadata: %d %v", count, err)
	}
}

func TestARetractedHumanCaptionDoesNotGainASystemOrigin(t *testing.T) {
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Proposal removed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE notice SET retracted_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": id}); err != nil {
		t.Fatal(err)
	}
	var metadata *systemtext.Message
	if err := e.owner.QueryRow(context.Background(), `SELECT system_text FROM notice WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if got := noticeSystemDisplay(systemtext.WithLocale(e.asUser(e.recipient), textlang.German), metadata, "Proposal removed", ""); got != nil {
		t.Fatal("retraction plus English equality reclassified original wording")
	}
}
