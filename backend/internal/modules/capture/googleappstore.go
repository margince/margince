// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The write path for the installation's Google OAuth app.
//
// The same shape ai.ProviderKeyStore uses for a BYOK credential, and for the
// same reasons: seal before recording, retire the superseded blob only after the
// record commits, and hold the settings write lock across the read that computes
// it. Not shared with that store — it moves ONE provider's ref in a map, this
// moves a pair — but the mechanics that must not diverge are the settings
// layer's own (settings.LockForWrite, keyvault.DeleteDetached), and both reach
// for those rather than spelling them again.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GoogleAppStore reads and writes the installation's Google OAuth app.
type GoogleAppStore struct {
	settings *settings.Store
	vault    keyvault.Vault
	// log carries the one failure this store cannot report to its caller: a
	// superseded secret it could not delete after the write already committed.
	log *slog.Logger
}

// NewGoogleAppStore builds the store over the settings catalog and the vault.
// A nil vault is a role that seals nothing: a write refuses rather than
// recording a ref to a secret that was never written.
func NewGoogleAppStore(s *settings.Store, vault keyvault.Vault, log *slog.Logger) *GoogleAppStore {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &GoogleAppStore{settings: s, vault: vault, log: log}
}

// maxGoogleAppFieldBytes bounds each half, mirroring the contract's maxLength.
// Google's own values are far shorter; the ceiling exists so a caller cannot
// seal a megabyte and have it read back on every request.
const maxGoogleAppFieldBytes = 512

// ErrNoVault is the refusal when this installation seals nothing.
//
// Separate from a validation error because the fix is the operator's — the vault
// root key — and not anything the caller sent, which is what decides whether the
// surface answers 503 or 422.
var ErrNoVault = errors.New("this installation has no key vault configured, so a Google client secret cannot be stored: set the vault root key on the api and worker, then try again")

// GoogleAppStatus is what a reader is told about the app: enough to know
// whether Gmail can be connected, and never the secret.
type GoogleAppStatus struct {
	// Configured reports whether both halves are stored.
	Configured bool
	// ClientID is returned in the clear because it is not a secret — it travels
	// in every authorization redirect — and an operator needs to see WHICH app
	// their installation is using to check it against the Google console.
	ClientID string
}

// Read reports the app's status. Empty is a legitimate answer: an installation
// that has not set one up has not failed at anything.
func (s *GoogleAppStore) Read(ctx context.Context) (GoogleAppStatus, error) {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionRead); err != nil {
		return GoogleAppStatus{}, err
	}
	app, err := settings.Get(ctx, s.settings, GoogleAppSetting)
	if err != nil {
		return GoogleAppStatus{}, err
	}
	return GoogleAppStatus{Configured: app.Configured(), ClientID: app.ClientID}, nil
}

// Credentials resolves the app for USE — the client id and the unsealed secret.
//
// Separate from Read because the two answer different questions and must not be
// confused: Read tells a person what is configured, this hands the server what it
// needs to talk to Google. The secret it returns never reaches a response body —
// the connect transport uses it to exchange an authorization code and nothing
// serializes it.
//
// Gated like every other read of this setting. The caller that needs it is
// usually a REP connecting their own mailbox, who holds no capture grant — so
// compose calls this on a system-principal context, the same way the model
// binding is resolved for a completion whoever triggered it. That keeps the gate
// rather than declaring the entry ungated, and it names the actor in the trail.
//
// Reports ok=false for an installation with no app, which is not an error: the
// connect surface then says so instead of failing.
func (s *GoogleAppStore) Credentials(ctx context.Context) (clientID, clientSecret string, ok bool, err error) {
	// Stated here rather than left to settings.Get's own gate, which enforces the
	// same rule one package away where no reader of this method — and no fitness
	// gate walking this package — can see it. A system principal passes it
	// unconditionally, which is what makes the transport's call work; an
	// unentitled human is refused before a secret is unsealed for them.
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionRead); err != nil {
		return "", "", false, err
	}
	app, err := settings.Get(ctx, s.settings, GoogleAppSetting)
	if err != nil {
		return "", "", false, err
	}
	if !app.Configured() {
		return "", "", false, nil
	}
	if s.vault == nil {
		return "", "", false, ErrNoVault
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return "", "", false, err
	}
	secret, err := s.vault.Get(ctx, ws, keyvault.Ref(app.ClientSecretRef))
	if err != nil {
		// A ref that will not open is an installation whose vault root key
		// changed under it, and the honest report is that the app is unusable
		// rather than absent — the two have different remedies.
		return "", "", false, fmt.Errorf("capture: the sealed Google client secret could not be opened: %w", err)
	}
	return app.ClientID, string(secret), true, nil
}

// Set stores the app, sealing the secret and replacing whatever was there.
//
// The new secret is sealed BEFORE the ref moves and the old one is retired only
// after the move commits, so no window exists in which the recorded ref names
// nothing. A failure part-way leaves the previous app serving.
func (s *GoogleAppStore) Set(ctx context.Context, clientID, clientSecret string) error {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return err
	}
	// Trimmed because both halves are pasted from the Google console, and a
	// value carrying a trailing newline authenticates nothing while looking
	// exactly like one that would.
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	// The contract declares maxLength on both, and nothing generated enforces it:
	// oapi-codegen emits no validation, and httperr.Decode caps only the whole
	// body. Without this a ~1 MiB "secret" is sealed and a ~1 MiB client id is
	// stored and handed back on every read.
	if len(clientID) > maxGoogleAppFieldBytes || len(clientSecret) > maxGoogleAppFieldBytes {
		return settings.InvalidValue{
			Setting: GoogleAppKey, Code: settings.CodeInvalidValue,
			Reason: fmt.Sprintf("a Google client id and secret are each at most %d bytes", maxGoogleAppFieldBytes),
		}
	}
	if clientID == "" || clientSecret == "" {
		return settings.InvalidValue{
			Setting: GoogleAppKey, Code: settings.CodeInvalidValue,
			Reason: "a Google app needs both the client id and the client secret; remove the app instead of storing half of one",
		}
	}
	// Shape-checked here as well as in the entry's validator, so a mistyped id is
	// refused before a secret is sealed for it — the validator runs on the
	// settings write, by which point the blob exists and would need retiring.
	if err := validateClientID(clientID); err != nil {
		return settings.InvalidValue{
			Setting: GoogleAppKey, Code: settings.CodeInvalidValue, Reason: err.Error(),
		}
	}
	if s.vault == nil {
		return ErrNoVault
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return err
	}
	ref, err := s.vault.Put(ctx, ws, []byte(clientSecret))
	if err != nil {
		return fmt.Errorf("capture: sealing the Google client secret: %w", err)
	}
	previous, err := s.swap(ctx, ws, GoogleApp{ClientID: clientID, ClientSecretRef: string(ref)}, string(ref))
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, "google app rotated")
	return nil
}

// Remove clears the app. Removing one that was never there succeeds: the caller
// asked for a state, and that state already holds.
func (s *GoogleAppStore) Remove(ctx context.Context) error {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return err
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return err
	}
	previous, err := s.swap(ctx, ws, GoogleApp{}, "")
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, "google app removed")
	return nil
}

// swap writes the app inside a single locked transaction and returns the secret
// ref it displaced.
//
// The lock is taken before the READ for the same reason ai.ProviderKeyStore
// takes it there: the displaced ref is computed from the current value, so a
// read outside the write's transaction is a lost update — two admins rotating at
// once would each retire a blob the other still references.
func (s *GoogleAppStore) swap(ctx context.Context, ws ids.WorkspaceID, next GoogleApp, sealed string) (previous string, err error) {
	// Separated from the transaction's own error so the two can be told apart
	// below; they mean different things about what committed.
	var (
		entered bool
		bodyErr error
	)
	txErr := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		entered = true
		if bodyErr = settings.LockForWrite(ctx, tx, GoogleAppKey); bodyErr != nil {
			return bodyErr
		}
		var current GoogleApp
		if current, bodyErr = settings.GetTx(ctx, tx, GoogleAppSetting); bodyErr != nil {
			return bodyErr
		}
		previous = current.ClientSecretRef
		bodyErr = settings.SetTx(ctx, s.settings, tx, GoogleAppSetting, next)
		return bodyErr
	})
	if txErr == nil {
		return previous, nil
	}
	// The closure ran and returned nil, yet the call failed: the only thing left
	// is COMMIT, and a commit that reports failure may still have committed.
	// Deleting the newly sealed secret then would leave the stored ref naming a
	// blob that is gone — a Gmail integration that cannot be repaired by
	// re-entering the secret, because the app would still read as configured.
	// Unreferenced ciphertext is inert; a dangling ref is not.
	if entered && bodyErr == nil && sealed != "" {
		s.log.WarnContext(ctx,
			"a Google client secret was sealed but the settings transaction did not report a clean commit; the blob is KEPT because the write may have landed, so it is either live or an orphan to reconcile",
			"credential_ref", sealed, "err", txErr)
		return "", txErr
	}
	if sealed != "" {
		s.retire(ctx, ws, sealed, "google app write refused")
	}
	return "", txErr
}

// retire deletes a secret nothing references any more, through the shared helper
// for a detached credential.
func (s *GoogleAppStore) retire(ctx context.Context, ws ids.WorkspaceID, ref, lifecycle string) {
	if ref == "" || s.vault == nil {
		return
	}
	keyvault.DeleteDetached(ctx, s.vault, s.log, ws.UUID, keyvault.Ref(ref), lifecycle)
}

// credentialWorkspace is the caller's tenant, or a refusal. A credential write
// outside a workspace has no tenant to belong to, and the vault would refuse the
// ref later anyway — so it is refused here, where the message can say why.
func credentialWorkspace(ctx context.Context) (ids.WorkspaceID, error) {
	raw, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.WorkspaceID{}, errors.New("capture: a Google app write outside a workspace context")
	}
	return ids.From[ids.WorkspaceKind](raw), nil
}
