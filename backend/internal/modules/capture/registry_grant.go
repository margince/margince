// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Granting a connector: the one path by which a human lends a connector their
// authority over their own mailbox. Everything the grant has to establish
// before a credential is stored lives here — live authority, the scope
// intersection, what the provider says it granted — plus the single upsert
// that writes (or re-points) the connection row.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Connect grants one connector under the CALLING human's authority. Two
// guards run here, both at grant time rather than discovered at 3am
// mid-sync: the granting human must still resolve as live authority, and
// a connector demanding scopes they do not hold is refused. Mail sharing
// is a workspace SETTING (capture.mail_sharing, ON by default) rather
// than a per-connect acknowledgment; share_acknowledged_at records when
// this grant connected under that regime.
//
// A grant for an account the caller already connected lands on that row, so
// connecting one account twice never makes a second mailbox.
//
// note: the returned id (and the connectionID threaded through SyncOnce and
// the sync-state recording) names a capture_connection row, which the kernel
// does not model as a first-class entity — no kind exists for it, so it stays
// ids.UUID rather than inventing one.
func (r *Registry) Connect(ctx context.Context, name string, auth connector.Auth) (ids.UUID, error) {
	return r.Grant(ctx, name, auth, ids.Nil)
}

// ErrReconnectOtherAccount refuses a reconnect whose grant names a different
// account than the mailbox holds. That account is another mailbox, which the
// caller adds instead. The old row's watermark, posture and baseline are not
// its to inherit.
var ErrReconnectOtherAccount = fmt.Errorf("the account you signed in with is not this mailbox's; add it as another mailbox instead: %w",
	apperrors.ErrConflict)

// Grant is Connect when target is zero. Otherwise it grants again over target,
// the caller's own connection that a reauth banner asks them to repair. It
// refuses a grant for another account.
func (r *Registry) Grant(ctx context.Context, name string, auth connector.Auth, target ids.UUID) (ids.UUID, error) {
	c, err := r.connector(name)
	if err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ids.Nil, fmt.Errorf("only a human grants a connector: %w", apperrors.ErrPermissionDenied)
	}
	scopes, err := grantedScopes(c, actor, name)
	if err != nil {
		return ids.Nil, err
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.Nil, errors.New("capture: connector grant outside workspace context")
	}
	if r.vault == nil {
		return ids.Nil, errors.New("capture: no keyvault configured — a connector credential cannot be sealed")
	}
	// A grant lends the granting human's authority to a connector, so that
	// authority has to be live at the moment it is lent — the principal
	// reaching here was built by a transport from a credential minted earlier,
	// and a human deactivated in between must not still be able to lend it.
	// Checked in the registry, the one place every transport passes through, so
	// the invariant does not rest on each caller's own check. It runs before
	// the seal below, so a refusal leaves neither a row nor a stored secret.
	// The same check refuses a member whose role cannot record what capture
	// writes, so no mailbox is connected only to fail on its first sync.
	if err := r.requireCapturingGrantor(ctx, ws, actor.UserID); err != nil {
		return ids.Nil, err
	}
	// Put-then-commit (like blobstore): seal the credential in the vault
	// first, then commit the row that names it. The row stores the opaque ref;
	// the bytes never touch it. A refusal inside the transaction deletes the
	// blob. A transaction that fails for any other reason strands it, inert and
	// encrypted, with no vault sweep to collect it.
	ref, err := r.vault.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(auth))
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: sealing connector credential: %w", err)
	}
	row := connectionUpsert{
		userID:         actor.UserID,
		provider:       name,
		scopes:         scopes,
		ref:            ref,
		accountLabel:   accountLabelFor(ctx, c, auth, name),
		providerScopes: providerScopesFor(ctx, c, auth, name),
	}
	var id ids.UUID
	var priorRef *string
	refused := false
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, priorRef, refused, err = r.grantTx(ctx, tx, row, target)
		return err
	}); err != nil {
		if refused {
			keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws, ref, "refused connect")
		}
		if errors.Is(err, ErrNoConnection) {
			return ids.Nil, err
		}
		return ids.Nil, fmt.Errorf("capture: storing connection: %w", err)
	}
	// The row now names the fresh ref; a prior ref (a genuine reconnect over
	// an existing row) is unreachable from any row from here on and must be
	// destroyed — the same invariant Disconnect enforces, on the overwrite
	// path rather than the withdraw path. A first-time connect has no prior
	// ref: nothing to delete. The delete runs AFTER commit (put-then-commit's
	// mirror: the row is already safely repointed at the new secret before the
	// old one is destroyed), so it must outlive the request and must not fail
	// it — the reconnect is committed and there is nothing left to undo.
	if priorRef != nil {
		keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws, keyvault.Ref(*priorRef), "reconnect")
	}
	return id, nil
}

// grantTx writes one grant's row under the locks it needs. refused reports a
// refusal, after which the caller destroys the credential it sealed.
func (r *Registry) grantTx(ctx context.Context, tx pgx.Tx, row connectionUpsert, target ids.UUID) (ids.UUID, *string, bool, error) {
	// Asked again before any row lock and held until commit: a demotion
	// since the check above either waits for this row or refuses it.
	if _, err := r.sink.captureAllowedTx(ctx, tx, row.userID); err != nil {
		return ids.Nil, nil, true, err
	}
	// The same lock the OAuth-app write takes, before anything counts this
	// row. refuseStrandingConnections reads the connection count under it and
	// then commits the new client id. Without it this row could land between
	// the two, connected against a client id that no longer exists.
	if key := appSettingKeyForConnector(row.provider); key != "" {
		if err := settings.LockForWrite(ctx, tx, key); err != nil {
			return ids.Nil, nil, false, err
		}
	}
	if err := lockConnectionClaimsTx(ctx, tx, row.userID, target, row.accountLabel); err != nil {
		return ids.Nil, nil, false, err
	}
	id, priorRef, err := upsertConnection(ctx, tx, row, target)
	return id, priorRef, errors.Is(err, ErrReconnectOtherAccount) || errors.Is(err, ErrNoConnection), err
}

// requireCapturingMember is requireCapturingGrantor for the member a history
// import is asked for, in the caller's workspace.
func (r *Registry) requireCapturingMember(ctx context.Context, userID ids.UserID) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("capture: a history import outside workspace context")
	}
	return r.requireCapturingGrantor(ctx, ws, userID.UUID)
}

// requireCapturingGrantor resolves the granting human against identity's live
// authority and asks CaptureRefusal about it. An archived, suspended, or
// deactivated human resolves to ErrNotFound, which is the refusal. Nothing is
// sealed or written first, so nothing has to be undone. Starting a history
// import asks it too.
func (r *Registry) requireCapturingGrantor(ctx context.Context, ws, userID ids.UUID) error {
	if r.authority == nil {
		return errors.New("capture: no authority resolver configured — a connector grant cannot be checked against live authority")
	}
	rbac, err := r.authority.EffectiveRBAC(ctx, ws, userID)
	if err != nil {
		return fmt.Errorf("capture: the granting human's authority does not resolve: %w", err)
	}
	seat, err := r.authority.SeatType(ctx, ws, userID)
	if err != nil {
		return fmt.Errorf("capture: the granting human's seat does not resolve: %w", err)
	}
	for _, object := range captureWriterGrants {
		create := auth.Check{Object: object, Actions: []principal.Action{principal.ActionCreate}}
		auth.RecordDecision(ctx, create, rbac.Permissions.Allows(object, principal.ActionCreate))
	}
	return CaptureRefusal(rbac.Permissions, seat)
}

// grantedScopes runs the grant-time scope intersection and returns the
// connector's declared scopes as the strings the row freezes. A connector
// demanding a scope the granting human does not hold is refused here rather
// than discovered at 3am mid-sync.
func grantedScopes(c connector.Connector, actor principal.Principal, name string) ([]string, error) {
	declared := c.Descriptor().Scopes
	out := make([]string, 0, len(declared))
	for _, scope := range declared {
		if !actor.Scopes.Has(scope) {
			return nil, fmt.Errorf("the connector %s needs the scope %q, which the granting human does not hold: %w",
				name, scope, apperrors.ErrScopeExceeded)
		}
		out = append(out, string(scope))
	}
	return out, nil
}

// accountLabelFor asks the connector to name the account it just authorized.
// Display-only; a connector that cannot name its account simply does not
// implement the seam. This must not fail the connect — a missing label is a
// blank line in the UI, not a lost connection.
func accountLabelFor(ctx context.Context, c connector.Connector, auth connector.Auth, name string) *string {
	labeler, ok := c.(connector.AccountLabeler)
	if !ok {
		return nil
	}
	label, err := labeler.AccountLabel(auth)
	if err != nil {
		slog.WarnContext(ctx, "capture: connector could not name its account", "provider", name, "err", err)
		return nil
	}
	if label == "" {
		return nil
	}
	return &label
}

// providerScopesFor asks the connector what the PROVIDER granted — the
// provider's own vocabulary, distinct from the internal scopes the row freezes
// alongside it. A connector with no consent step to read implements no such
// seam, and a connector that cannot parse its own bundle is a bug worth a log
// line, not a reason to refuse a grant the human already completed: both leave
// the column NULL, which reads as "not known" rather than "nothing granted".
func providerScopesFor(ctx context.Context, c connector.Connector, auth connector.Auth, name string) []string {
	scoper, ok := c.(connector.GrantedScoper)
	if !ok {
		return nil
	}
	granted, err := scoper.GrantedScopes(auth)
	if err != nil {
		slog.WarnContext(ctx, "capture: connector could not report its granted scopes", "provider", name, "err", err)
		return nil
	}
	return granted
}

// otherAccount reports whether a reconnect's grant names a DIFFERENT provider
// account than the row holds. Both labels must be known: a connector that
// cannot name its account reports nothing, and nothing is not evidence of a
// change. Addresses compare without case, as the unique key does.
func otherAccount(prior, next *string) bool {
	if prior == nil || *prior == "" || next == nil || *next == "" {
		return false
	}
	return !strings.EqualFold(*prior, *next)
}

// connectionUpsert is one connect transaction's write: the granting human, the
// provider, the internal scopes frozen at grant time and the provider scopes
// actually granted, the ref naming the freshly sealed credential, and the
// display-only account label.
type connectionUpsert struct {
	userID   ids.UUID
	provider string
	scopes   []string
	// providerScopes is the provider's own vocabulary; nil when the connector
	// cannot report it, and stored as NULL rather than an empty list so
	// "unknown" never reads as "nothing".
	providerScopes []string
	ref            keyvault.Ref
	accountLabel   *string
}

// grantSet is what a grant writes over a row it lands on. A label the
// connector could not read keeps the one the row holds.
const grantSet = `credential_ref = @ref, auth = NULL, status = 'connected', archived_at = NULL, connected_at = now(),
	share_acknowledged_at = COALESCE(capture_connection.share_acknowledged_at, now()),
	account_label = COALESCE(@label, capture_connection.account_label), provider_scopes = @provider_scopes`

// memberMailboxConflict is the unique key one member's account lands on.
const memberMailboxConflict = `(user_id, provider, lower(account_label)) WHERE user_id IS NOT NULL`

// upsertConnection writes (or re-points) one of the caller's connection rows
// and audits the change inside the connect transaction. It returns the row id
// and the credential ref this write superseded, nil for a first-time connect.
// The caller destroys that ref once the transaction has committed.
func upsertConnection(ctx context.Context, tx pgx.Tx, in connectionUpsert, target ids.UUID) (ids.UUID, *string, error) {
	prior, err := priorGrantTx(ctx, tx, in, target)
	if err != nil {
		return ids.Nil, nil, err
	}
	if !target.IsZero() && otherAccount(prior.label, in.accountLabel) {
		return ids.Nil, nil, ErrReconnectOtherAccount
	}
	granted := pgx.StrictNamedArgs{"ref": string(in.ref), "label": in.accountLabel, "provider_scopes": in.providerScopes}
	var id ids.UUID
	if prior.id.IsZero() {
		granted[sqlArgProvider], granted["user"], granted["scopes"] = in.provider, in.userID, in.scopes
		err = tx.QueryRow(ctx, `
		INSERT INTO capture_connection (provider, user_id, scopes, credential_ref, status, account_label, provider_scopes, share_acknowledged_at, legal_entity_id)
		VALUES (@provider, @user, @scopes, @ref, 'connected', @label, @provider_scopes, now(), `+storekit.LegalEntityOfSQL("@user")+`)
		ON CONFLICT `+memberMailboxConflict+` DO UPDATE SET `+grantSet+`
		RETURNING id`, granted).Scan(&id)
	} else {
		granted["id"] = prior.id
		err = tx.QueryRow(ctx, `UPDATE capture_connection SET `+grantSet+` WHERE id = @id RETURNING id`, granted).Scan(&id)
	}
	if err != nil {
		return ids.Nil, nil, err
	}
	database.Forget(tx, ownerIdentitiesMemo)
	if err := auditConnectedAccountTx(ctx, tx, id, in, prior.status, prior.label); err != nil {
		return ids.Nil, nil, err
	}
	if err := resetConnectionSchedule(ctx, tx, id); err != nil {
		return ids.Nil, nil, err
	}
	return id, prior.ref, nil
}

// priorGrant is the row a grant lands on, as it stood; a zero id is none.
type priorGrant struct {
	id                 ids.UUID
	ref, status, label *string
}

// priorGrantTx locks the row a grant lands on. That is the target of a
// reconnect, which must be the caller's own, or the added account's own row.
// The FOR UPDATE lock serializes a concurrent disconnect or reconnect.
// Without the credential_ref read the write would orphan the previous secret
// in the vault; status and label are the audit before-image.
func priorGrantTx(ctx context.Context, tx pgx.Tx, in connectionUpsert, target ids.UUID) (priorGrant, error) {
	var prior priorGrant
	match, args := `lower(account_label) IS NOT DISTINCT FROM lower($3::text)`, []any{in.userID, in.provider, in.accountLabel}
	if !target.IsZero() {
		match, args = `id = $3`, []any{in.userID, in.provider, target}
	}
	err := tx.QueryRow(ctx, `SELECT id, credential_ref, status, account_label FROM capture_connection
		 WHERE user_id = $1 AND provider = $2 AND `+match+` FOR UPDATE`, args...).
		Scan(&prior.id, &prior.ref, &prior.status, &prior.label)
	switch {
	case errors.Is(err, pgx.ErrNoRows) && !target.IsZero():
		return prior, ErrNoConnection
	case errors.Is(err, pgx.ErrNoRows):
		return prior, nil
	}
	return prior, err
}

func auditConnectedAccountTx(ctx context.Context, tx pgx.Tx, id ids.UUID, in connectionUpsert, priorStatus, priorLabel *string) error {
	// A grant is a human's deliberate act over their own mailbox, so it is
	// attributed like any other record mutation. A row that already existed is
	// a reconnect (an update over the same connection), never a second create.
	// The images carry the connection, never the credential ref or auth bytes.
	verb, before := "create", map[string]any(nil)
	if priorStatus != nil {
		verb = "update"
		before = connectionAuditImage(in.provider, *priorStatus, priorLabel)
	}
	return auditLifecycle(ctx, tx, verb, captureConnectionObject, id, before,
		connectionAuditImage(in.provider, "connected", in.accountLabel))
}

// resetConnectionSchedule starts the scheduling ladder clean: a row parked by
// reauth_required or degraded by backoff is due immediately with a fresh
// credential (ADR-0063).
func resetConnectionSchedule(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE capture_sync_state
		SET next_sync_at = now(), consecutive_failures = 0, last_error_class = NULL, failing_since = NULL
		WHERE connection_id = $1`, id)
	return err
}
