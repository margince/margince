// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type medicalDocumentBrain struct {
	question ids.UUID
	kind     string
	calls    int
}

func (b *medicalDocumentBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.calls++
	return model.Response{Text: fmt.Sprintf(`{"results":[{"id":%q,"verdict":%q,"confidence":0.95}]}`, b.question.String(), b.kind)}, nil
}

func TestAnUnreadMedicalDocumentKeepsItsPersonalClassification(t *testing.T) {
	for _, kind := range []string{"personal", "ordinary"} {
		t.Run(kind, func(t *testing.T) {
			blob := blobstore.NewMemory()
			env := newCaptureEnvWithBlob(t, captureNow, blob)
			e, sync := env.e, env.sync
			allowSharedPosture(t, e)
			setPosture(t, env, e.Rep1, capturemod.PostureClassified)
			const sender, key = "doctor@health.example", "medical-file@health.example"
			clearSender(t, e, sender)
			raw := emailWithFile(sender, captureOwner, key, "", "Befund.jpg", []byte{0xff, 0xd8, 0xff, 0xe0})
			raw = []byte(strings.Replace(string(raw), "please find attached", "My medical test results", 1))
			sync(t, raw)
			activity := activityIDOf(t, e, key)
			var question ids.UUID
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(t.Context(), `SELECT id FROM capture_thread_verdict WHERE first_activity_id=@activity AND user_id=@seat`, pgx.StrictNamedArgs{"activity": activity, "seat": e.Rep1}).Scan(&question)
			}); err != nil {
				t.Fatal(err)
			}
			brain := &medicalDocumentBrain{question: question, kind: kind}
			engine := compose.NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
			if err := engine.RunWorkspace(principal.WithWorkspaceID(t.Context(), e.WS), 0); err != nil {
				t.Fatal(err)
			}
			var status, storedKind string
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(t.Context(), `SELECT status,kind FROM capture_thread_verdict WHERE id=@question`, pgx.StrictNamedArgs{"question": question}).Scan(&status, &storedKind)
			}); err != nil {
				t.Fatal(err)
			}
			if storedKind != kind || brain.calls != 1 {
				t.Fatalf("kind=%s calls=%d, want model's %s", storedKind, brain.calls, kind)
			}
			want := capturemod.VerdictHeld
			if kind == "ordinary" {
				want = capturemod.VerdictUnsure
			}
			if status != want {
				t.Fatalf("unread medical file status=%s, want %s", status, want)
			}
			if got, _ := audienceOf(t, e, activity); got != "participants" {
				t.Fatalf("medical file audience=%s", got)
			}
			if kind == "personal" {
				sync(t, emailWithFile(sender, captureOwner, "later-medical@health.example", key, "test_result.jpg", []byte{0xff, 0xd8, 0xff, 0xe0}))
				later := activityIDOf(t, e, "later-medical@health.example")
				var withheld int
				if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
					return tx.QueryRow(t.Context(), `SELECT count(*) FROM attachment WHERE activity_id=@activity AND bytes_withheld AND storage_key=''`, pgx.StrictNamedArgs{"activity": later}).Scan(&withheld)
				}); err != nil {
					t.Fatal(err)
				}
				if withheld != 1 {
					t.Fatalf("personal classification did not withhold the next file: %d", withheld)
				}
			}
		})
	}
}
