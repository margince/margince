// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/base64"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"strings"
	"testing"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
)

// emailWithFile is email() carrying one attached file.
func emailWithFile(from, to, msgID, refs, filename string, file []byte) []byte {
	lines := []string{
		"From: " + from,
		"To: " + to,
		"Subject: project",
		"Date: Wed, 04 Jun 2026 09:00:00 +0000",
		"Message-ID: <" + msgID + ">",

		`Content-Type: multipart/mixed; boundary="part"`,
		"",
		"--part",
		"Content-Type: text/plain",
		"",
		"please find attached",
		"--part",
		`Content-Type: application/octet-stream; name="` + filename + `"`,
		`Content-Disposition: attachment; filename="` + filename + `"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString(file),
		"--part--",
		"",
	}
	if refs != "" {
		lines = append([]string{"References: <" + refs + ">"}, lines...)
	}
	return []byte(strings.Join(lines, "\r\n"))
}

func TestASeenSendersReplyCarryingADocumentIsJudgedAgain(t *testing.T) {
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	seedThreadVerdict(t, e, e.Rep1, "thread-root@vendor.example", "cleared", []string{"billing@vendor.example"})

	// The sender the verdict saw replies with a document it never read.
	sync(t, emailWithFile("billing@vendor.example", captureOwner, "file-reply@vendor.example",
		"thread-root@vendor.example", "Cost+ VER 2.csv", []byte("Employee,Stack,Jan\nAn Bui,Senior Frontend,4990\n")))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "file-reply@vendor.example")); got != "participants" {
		t.Fatalf("a reply carrying an unread document is %q, want participants", got)
	}
	if got := threadVerdictStatus(t, e, e.Rep1, "thread-root@vendor.example"); got != "pending" {
		t.Fatalf("the ledger is %q after a reply with a document, want pending so the file is read", got)
	}

	// An owner's share covered the messages they saw, not a file sent after it.
	seedThreadVerdict(t, e, e.Rep1, "thread-shared@vendor.example", "shared_by_owner", []string{"billing@vendor.example"})
	sync(t, emailWithFile("billing@vendor.example", captureOwner, "shared-file-reply@vendor.example",
		"thread-shared@vendor.example", "Cost+.xlsx", []byte("PK\x03\x04")))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "shared-file-reply@vendor.example")); got != "participants" {
		t.Fatalf("a file sent after the owner shared the thread is %q, want participants", got)
	}

	// A reply from the same sender with no document still takes the answer.
	seedThreadVerdict(t, e, e.Rep1, "thread-other@vendor.example", "cleared", []string{"billing@vendor.example"})
	sync(t, email("billing@vendor.example", "", captureOwner, "plain-reply@vendor.example", "thread-other@vendor.example"))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "plain-reply@vendor.example")); got != "workspace" {
		t.Fatalf("a plain reply from a seen sender is %q, want workspace", got)
	}
}

func TestAClearedSendersNewDocumentWaitsForItsOwnVerdict(t *testing.T) {
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	const sender = "buyer@vendor.example"
	clearSender(t, e, sender)
	for _, filename := range []string{"proposal.csv", "passport.png"} {
		t.Run(filename, func(t *testing.T) {
			key := "new-document-" + filename + "@vendor.example"
			sync(t, emailWithFile(sender, captureOwner, key, "", filename, []byte("Document details")))
			id := activityIDOf(t, e, key)
			if got, _ := audienceOf(t, e, id); got != "participants" {
				t.Fatalf("a cleared sender's new document was born %q", got)
			}
			assertDocumentQuestion(t, env, id)
		})
	}
	sync(t, email(sender, "Buyer", captureOwner, "plain-cleared@vendor.example", ""))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "plain-cleared@vendor.example")); got != "workspace" {
		t.Fatalf("ordinary mail from a cleared sender is %q", got)
	}
}

func TestAReopenedThreadHoldsRepliesWhileItsDocumentIsPending(t *testing.T) {
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	const sender, root = "buyer@vendor.example", "pending-doc@vendor.example"
	clearSender(t, e, sender)
	seedThreadVerdict(t, e, e.Rep1, root, "cleared", []string{sender})
	sync(t, emailWithFile(sender, captureOwner, "new-file@vendor.example", root, "proposal.csv", []byte("Price,Total\nWork,10\n")))
	document := activityIDOf(t, e, "new-file@vendor.example")
	if got, _ := audienceOf(t, e, document); got != "participants" {
		t.Fatalf("document reply was born %q", got)
	}
	sync(t, email(sender, "Buyer", captureOwner, "pending-reply@vendor.example", root))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "pending-reply@vendor.example")); got != "participants" {
		t.Fatalf("a reply bypassed its pending document verdict: %q", got)
	}
	assertDocumentQuestion(t, env, document)
}

func assertDocumentQuestion(t *testing.T, env captureEnv, activity ids.UUID) {
	t.Helper()
	e := env.e
	var pointed ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT first_activity_id FROM capture_thread_verdict
   WHERE user_id=@seat AND thread_key=(SELECT thread_key FROM activity WHERE id=@activity) AND status='pending'`, pgx.StrictNamedArgs{"seat": e.Rep1, "activity": activity}).Scan(&pointed)
	}); err != nil {
		t.Fatal(err)
	}
	if pointed != activity {
		t.Fatalf("pending question points at %s, want document %s", pointed, activity)
	}
	rows, err := capturemod.NewThreadVerdictStore(compose.InstallationDB(e.Pool)).ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ActivityID == activity {
			if len(row.Attachments) == 0 || len(row.Documents) == 0 {
				t.Fatal("the classifier cannot see the attachment")
			}
			return
		}
	}
	t.Fatal("the document's question cannot be claimed")
}

func TestASharedMailboxDoesNotInheritAnIdlePendingQuestion(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)
	const sender, root = "buyer@vendor.example", "switch-posture@vendor.example"
	clearSender(t, e, sender)
	sync(t, emailWithFile(sender, captureOwner, root, "", "proposal.csv", []byte("Price,Total\nWork,10\n")))
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	sync(t, email(sender, "Buyer", captureOwner, "shared-after-pending@vendor.example", root))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "shared-after-pending@vendor.example")); got != "workspace" {
		t.Fatalf("shared mailbox's reply is %q behind an idle question", got)
	}
}

func TestASharedMailboxDoesNotCommissionADocumentVerdict(t *testing.T) {
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	const sender, root = "buyer@vendor.example", "shared-doc@vendor.example"
	clearSender(t, e, sender)
	seedThreadVerdict(t, e, e.Rep1, root, "cleared", []string{sender})
	sync(t, emailWithFile(sender, captureOwner, "shared-doc-reply@vendor.example", root, "proposal.csv", []byte("Price,Total\nWork,10\n")))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "shared-doc-reply@vendor.example")); got != "workspace" {
		t.Fatalf("a shared mailbox's document waited for an unclaimable verdict: %s", got)
	}
}
