// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"encoding/base64"
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
		"References: <" + refs + ">",
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
	return []byte(strings.Join(lines, "\r\n"))
}

func TestASeenSendersReplyCarryingADocumentIsJudgedAgain(t *testing.T) {
	env := newCaptureEnv(t)
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
