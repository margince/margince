// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// winmailOnly is the Outlook shape whose text sits inside winmail.dat, which
// only a mail program can open.
func winmailOnly(msgID string) []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>",
		"To: " + captureOwner,
		"Subject: AW: Angebot",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="W"`,
		"",
		"--W",
		"Content-Type: application/ms-tnef",
		`Content-Disposition: attachment; filename="winmail.dat"`,
		"",
		"eJ8+IgAAAQ==",
		"--W--",
		"",
	}, "\r\n"))
}

// The completeness is stored with the row. The email drawer's read and the
// activity read serve it to a reader of the body. A reader the content is
// withheld from learns nothing of it.
func TestAMailWhoseTextCouldNotBeReadSaysSo(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, winmailOnly("winmail-1@acme.example"))
	activityID := oneActivityID(t, e)

	var stored string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body_completeness FROM activity WHERE id = $1`, activityID).Scan(&stored)
	}); err != nil {
		t.Fatalf("reading the stored completeness: %v", err)
	}
	if stored != "unreadable" {
		t.Fatalf("activity.body_completeness = %q for a mail whose text is inside winmail.dat", stored)
	}

	store := activities.NewStore(e.DB())
	id := ids.From[ids.ActivityKind](activityID)
	owner := readerCtx(e, e.Rep1)
	shown, err := store.GetEmailPresentation(owner, id, nil)
	if err != nil {
		t.Fatalf("the owner's email read: %v", err)
	}
	if shown.BodyCompleteness == nil || *shown.BodyCompleteness != crmcontracts.BodyCompletenessUnreadable {
		t.Errorf("email presentation body_completeness = %v, want unreadable", shown.BodyCompleteness)
	}
	row, err := store.GetActivity(owner, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("the owner's activity read: %v", err)
	}
	if row.BodyCompleteness == nil || *row.BodyCompleteness != crmcontracts.BodyCompletenessUnreadable {
		t.Errorf("activity body_completeness = %v, want unreadable", row.BodyCompleteness)
	}

	// Filed under no record, so the mail is held to its participants.
	withheld, err := store.GetEmailPresentation(readerCtx(e, e.Rep3), id, nil)
	if err != nil {
		t.Fatalf("a colleague's email read: %v", err)
	}
	if withheld.Access.ContentState != crmcontracts.EmailAccessContentStateWithheld {
		t.Fatalf("content_state = %v for a colleague; the case needs a withheld reader", withheld.Access.ContentState)
	}
	if withheld.BodyCompleteness != nil {
		t.Errorf("a withheld reader was told body_completeness = %v", *withheld.BodyCompleteness)
	}
}
