// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
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

// mailboxRunKey marks a context the registry built for one mailbox sync, page or
// push. Its writes ask CaptureRefusal again inside their own transaction.
type mailboxRunKey struct{}

func asMailboxRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, mailboxRunKey{}, true)
}

type mailboxConnectionKey struct{}

type mailboxConnection struct {
	ID         ids.UUID
	Generation int
}

func withMailboxConnection(ctx context.Context, id ids.UUID, generation int) context.Context {
	return context.WithValue(ctx, mailboxConnectionKey{}, mailboxConnection{ID: id, Generation: generation})
}

func isMailboxRun(ctx context.Context) bool {
	run, ok := ctx.Value(mailboxRunKey{}).(bool)
	return ok && run
}

// withAuthority rebinds ctx's actor to authority read inside a transaction.
func withAuthority(ctx context.Context, rbac authz.RBAC, seat principal.SeatType) (context.Context, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx, apperrors.ErrPermissionDenied
	}
	actor.Permissions, actor.TeamIDs, actor.SeatType = rbac.Permissions, rbac.TeamIDs, seat
	return principal.WithActor(ctx, actor), nil
}

// captureAllowedTx asks CaptureRefusal of the member's authority as of tx. A
// role change that takes capture away then waits for this write or refuses it.
// Call it before tx locks a row. A sink composed without an authority
// reader asks nothing.
func (s *Sink) captureAllowedTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (context.Context, error) {
	if s == nil || s.partyAuthority == nil {
		return ctx, nil
	}
	rbac, seat, err := s.partyAuthority.HeldAuthorityTx(ctx, tx, user)
	if err != nil {
		return ctx, err
	}
	if ctx, err = withAuthority(ctx, rbac, seat); err != nil {
		return ctx, err
	}
	return ctx, actingCaptureRefusal(ctx)
}

// mailboxRunAllowedTx is captureAllowedTx for the member a mailbox run acts
// for. A capture from any other door is not asked.
func (s *Sink) mailboxRunAllowedTx(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	if !isMailboxRun(ctx) {
		return ctx, nil
	}
	// An archived mailbox births held mail without provider keys. The lock still
	// excludes writes from a different account generation.
	if connection, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection); bound {
		var generation int
		err := tx.QueryRow(ctx, `SELECT generation FROM capture_connection
 WHERE id=@id AND generation=@account_generation FOR SHARE`,
			pgx.NamedArgs{"id": connection.ID, "account_generation": connection.Generation}).Scan(&generation)
		if errors.Is(err, pgx.ErrNoRows) {
			return ctx, apperrors.ErrPermissionDenied
		}
		if err != nil {
			return ctx, err
		}
	}
	_, owner := capturePrincipal(ctx)
	return s.captureAllowedTx(ctx, tx, owner)
}
