// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The objects capture's writers create. The sink's own auth.Require calls name
// these, so the floor below and the writes it guards read one list.
const (
	capturedActivityObject = "activity"
	capturedContactObject  = "contact"
	capturedLeadObject     = "lead"
)

// captureWriterGrants lists the creates the sink's writers ask for. A role
// missing one of them would connect and then fail part-way through a sync.
var captureWriterGrants = []string{capturedActivityObject, capturedContactObject, capturedLeadObject}

// CodeRoleCannotCapture is the wire code of a refusal by CaptureRefusal.
const CodeRoleCannotCapture = "role_cannot_capture"

// RoleCannotCaptureError says the member's role or seat cannot record what
// capture writes. It answers 403 role_cannot_capture, and the problem carries
// `missing` so a client words the refusal in the reader's language.
type RoleCannotCaptureError struct {
	// Missing is the object whose create the member lacks, or seatMissing for
	// a read seat.
	Missing string
}

// seatMissing is the Missing value of a read seat.
const seatMissing = "seat"

// missingWords is how the sentence names each lacking grant.
var missingWords = map[string]string{
	capturedActivityObject: "activities", capturedContactObject: "contacts", capturedLeadObject: "leads",
}

func (e *RoleCannotCaptureError) Error() string {
	if e.Missing == seatMissing {
		return "Your seat is read-only, so this mailbox cannot be captured. Ask an admin for a full seat."
	}
	return "Your role cannot create " + missingWords[e.Missing] +
		", so this mailbox cannot be captured. Ask an admin to change your role."
}

func (e *RoleCannotCaptureError) Unwrap() error { return apperrors.ErrPermissionDenied }

// FaultReference hands the client what is missing.
func (e *RoleCannotCaptureError) FaultReference() map[string]any {
	return map[string]any{"missing": e.Missing}
}

// Unreferenced is the same refusal without the reference: it decides the
// status and the code.
func (e *RoleCannotCaptureError) Unreferenced() error { return roleCannotCaptureFault{e.Error()} }

// roleCannotCaptureFault is the refusal's status and code.
type roleCannotCaptureFault struct{ sentence string }

func (f roleCannotCaptureFault) Error() string { return f.sentence }

func (f roleCannotCaptureFault) Unwrap() error { return apperrors.ErrPermissionDenied }

// MessageFault names the code and the sentence the member reads.
func (f roleCannotCaptureFault) MessageFault() (code, message string) {
	return CodeRoleCannotCapture, f.sentence
}

// CaptureRefusal answers whether a member with these permissions and this seat
// may have a mailbox captured. Connect, sync, backfill and the role dialog all
// ask it. It takes permissions rather than a context because the OAuth
// callback and the role editor hold permissions no context carries.
func CaptureRefusal(perms principal.Permissions, seat principal.SeatType) error {
	if !seat.CanMutate() {
		return &RoleCannotCaptureError{Missing: seatMissing}
	}
	for _, object := range captureWriterGrants {
		if !perms.Allows(object, principal.ActionCreate) {
			return &RoleCannotCaptureError{Missing: object}
		}
	}
	return nil
}

// actingCaptureRefusal asks CaptureRefusal about the member a connector
// context acts for. A context with no actor is refused like a read seat.
func actingCaptureRefusal(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return &RoleCannotCaptureError{Missing: seatMissing}
	}
	return CaptureRefusal(actor.Permissions, actor.SeatType)
}

// syncCredential is the credential a sync or backfill page runs with, asked
// only once the member behind runCtx may still capture.
func (r *Registry) syncCredential(runCtx context.Context, credentialRef *string, authBytes []byte) (connector.Auth, error) {
	if err := actingCaptureRefusal(runCtx); err != nil {
		return nil, err
	}
	return r.resolveCredential(runCtx, credentialRef, authBytes)
}
