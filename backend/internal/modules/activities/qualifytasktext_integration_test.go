// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A qualify task written before system text gets its template, so a
// Vietnamese reader reads it in Vietnamese. A subject no template renders is
// left as it was.
func TestOlderQualifyTasksLearnTheirTemplate(t *testing.T) {
	e := setupLoad(t)
	source := provenance.LeadRepliedSource
	seed := func(subject string) ids.UUID {
		t.Helper()
		task, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{Kind: "task", Subject: &subject, Source: "system", SourceSystem: &source})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(task.Id)
	}
	canonical := seed("Review qualification for Lena Prospect")
	booked := seed("Review qualification for Lena Prospect after booking")
	held := seed("Review qualification for Lena Prospect after the meeting")
	other := seed("Call Lena about the budget")
	// Other tests in the package leave qualify tasks too, so every page is read.
	for after := ids.Nil; ; {
		var scanned []ids.UUID
		if err := database.WithWorkspaceTx(asClassifier(e), e.pool, func(tx pgx.Tx) error {
			var err error
			scanned, _, err = DescribeQualifyTaskTextTx(asClassifier(e), tx, after, 10)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(scanned) == 0 {
			break
		}
		after = scanned[len(scanned)-1]
	}
	read := func(id ids.UUID) *systemtext.Message {
		t.Helper()
		var message *systemtext.Message
		if err := e.owner.QueryRow(context.Background(), `SELECT system_text FROM activity WHERE id = $1`, id).Scan(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	for task, want := range map[ids.UUID]string{
		canonical: "Xem xét điều kiện của Lena Prospect",
		booked:    "Xem xét điều kiện của Lena Prospect sau khi đặt lịch",
		held:      "Xem xét điều kiện của Lena Prospect sau cuộc họp",
	} {
		described := read(task)
		if described == nil || described.Subject == nil {
			t.Fatalf("the qualify task that should read %q got no template", want)
		}
		if got, err := described.Subject.Render(textlang.Vietnamese); err != nil || got != want {
			t.Errorf("the Vietnamese reader reads %q (%v), want %q", got, err, want)
		}
	}
	if read(other) != nil {
		t.Error("a subject no template renders was given one")
	}
}
