// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The withheld-file writer refuses before it touches a row: a caller without
// create on activity, and a file whose category nobody derived. An empty list
// is not a refusal; it writes nothing. None of the three reaches the
// transaction, which is why a nil one is enough here.
func TestRecordWithheldFilesRefusesBeforeItWrites(t *testing.T) {
	store := &Store{}
	activity := ids.From[ids.ActivityKind](ids.NewV7())
	file := []WithheldFile{{PartID: "part:1", Filename: "payslip.pdf", ByteSize: 4096}}
	derived := CapturedFileSource{System: "imap", MessageID: "m-1", CapturedBy: "connector:imap", Category: "email_attachment"}

	readOnly := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:imap",
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"activity": {Read: true}}, RowScope: principal.RowScopeAll,
		},
	})
	if err := store.RecordWithheldFiles(readOnly, nil, activity, derived, file); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a caller without create on activity: %v, want ErrPermissionDenied", err)
	}
	if err := store.RecordWithheldFiles(capturePrincipalCtx(), nil, activity, derived, nil); err != nil {
		t.Errorf("no files: %v, want nothing written and no error", err)
	}
	underived := derived
	underived.Category = ""
	if err := store.RecordWithheldFiles(capturePrincipalCtx(), nil, activity, underived, file); !errors.Is(err, ErrCapturedFileCategoryMissing) {
		t.Errorf("an underived category: %v, want ErrCapturedFileCategoryMissing", err)
	}
}

// The send's refusal names the file and the field, and tells the sender to
// remove the file rather than attach it again.
func TestAWithheldAttachmentRefusalNamesTheFileAndTheField(t *testing.T) {
	refusal := &WithheldAttachmentError{Filename: "payslip.pdf"}
	field, code, message := refusal.FieldFault()
	if field != "attachment_ids" || code != "withheld_attachment" {
		t.Errorf("field fault = (%q, %q), want (attachment_ids, withheld_attachment)", field, code)
	}
	if message != refusal.Error() || message != `"payslip.pdf" came with private mail and was not kept, so it cannot be sent; remove it from this message` {
		t.Errorf("message = %q", message)
	}
}
