// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The write path for an installation's connector OAuth apps.
//
// The same shape ai.ProviderKeyStore uses for a BYOK credential, and for the
// same reasons: seal before recording, retire the superseded blob only after the
// record commits, and hold the settings write lock across the read that computes
// it. Not shared with that store — it moves ONE provider's ref in a map, this
// moves a pair — but the mechanics that must not diverge are the settings
// layer's own (settings.LockForWrite, keyvault.DeleteDetached), and both reach
// for those rather than spelling them again.
//
// One store for every provider. Which key a call lands under, and what a valid
// client id looks like, come from the appKind table in connectorapp.go; nothing
// else here knows Google from Microsoft.

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

// ConnectorAppStore reads and writes the installation's connector OAuth apps.
type ConnectorAppStore struct {
	settings *settings.Store
	vault    keyvault.Vault
	// log carries the one failure this store cannot report to its caller: a
	// superseded secret it could not delete after the write already committed.
	log *slog.Logger
}

// NewConnectorAppStore builds the store over the settings catalog and the vault.
// A nil vault is a role that seals nothing: a write refuses rather than
// recording a ref to a secret that was never written.
func NewConnectorAppStore(s *settings.Store, vault keyvault.Vault, log *slog.Logger) *ConnectorAppStore {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &ConnectorAppStore{settings: s, vault: vault, log: log}
}

// maxAppFieldBytes bounds each field, mirroring the contract's maxLength. The
// vendors' own values are far shorter; the ceiling exists so a caller cannot
// seal a megabyte and have it read back on every request.
const maxAppFieldBytes = 512

// ErrNoVault is the refusal when this installation seals nothing.
//
// Separate from a validation error because the fix is the operator's — the vault
// root key — and not anything the caller sent, which is what decides whether the
// surface answers 503 or 422.
var ErrNoVault = errors.New("this installation has no key vault configured, so a client secret cannot be stored: set the vault root key on the api and worker, then try again")

// ConnectorAppStatus is what a reader is told about an app: enough to know
// whether the provider can be connected, and never the secret.
type ConnectorAppStatus struct {
	// Configured reports whether both halves are stored.
	Configured bool
	// ClientID is returned in the clear because it is not a secret — it travels
	// in every authorization redirect — and an operator needs to see WHICH app
	// their installation is using to check it against the vendor's console.
	ClientID string
	// Tenant is the Entra directory a Microsoft app is pinned to, empty for an
	// app that authorizes any organization and for every Google app.
	Tenant string
}

// Read reports one provider's app status. Empty is a legitimate answer: an
// installation that has not set one up has not failed at anything.
func (s *ConnectorAppStore) Read(ctx context.Context, p AppProvider) (ConnectorAppStatus, error) {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionRead); err != nil {
		return ConnectorAppStatus{}, err
	}
	k, err := appKindFor(p)
	if err != nil {
		return ConnectorAppStatus{}, err
	}
	app, err := settings.Get(ctx, s.settings, k.entry)
	if err != nil {
		return ConnectorAppStatus{}, err
	}
	return ConnectorAppStatus{
		Configured: app.Configured(), ClientID: app.ClientID, Tenant: app.Tenant,
	}, nil
}

// Credentials resolves one provider's app for USE — the client id, the unsealed
// secret, and the directory it is pinned to.
//
// Separate from Read because the two answer different questions and must not be
// confused: Read tells a person what is configured, this hands the server what it
// needs to talk to the vendor. The secret it returns never reaches a response
// body — the connect transport uses it to exchange an authorization code and
// nothing serializes it.
//
// Gated like every other read of this setting. The caller that needs it is
// usually a REP connecting their own mailbox, who holds no capture grant — so
// compose calls this on a system-principal context, the same way the model
// binding is resolved for a completion whoever triggered it. That keeps the gate
// rather than declaring the entry ungated, and it names the actor in the trail.
//
// Reports ok=false for an installation with no app, which is not an error: the
// connect surface then says so instead of failing.
func (s *ConnectorAppStore) Credentials(ctx context.Context, p AppProvider) (ConnectorApp, bool, error) {
	// Stated here rather than left to settings.Get's own gate, which enforces the
	// same rule one package away where no reader of this method — and no fitness
	// gate walking this package — can see it. A system principal passes it
	// unconditionally, which is what makes the transport's call work; an
	// unentitled human is refused before a secret is unsealed for them.
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionRead); err != nil {
		return ConnectorApp{}, false, err
	}
	k, err := appKindFor(p)
	if err != nil {
		return ConnectorApp{}, false, err
	}
	app, err := settings.Get(ctx, s.settings, k.entry)
	if err != nil {
		return ConnectorApp{}, false, err
	}
	if !app.Configured() {
		return ConnectorApp{}, false, nil
	}
	if s.vault == nil {
		return ConnectorApp{}, false, ErrNoVault
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return ConnectorApp{}, false, err
	}
	secret, err := s.vault.Get(ctx, ws, keyvault.Ref(app.ClientSecretRef))
	if err != nil {
		// A ref that will not open is an installation whose vault root key
		// changed under it, and the honest report is that the app is unusable
		// rather than absent — the two have different remedies.
		return ConnectorApp{}, false, fmt.Errorf("capture: the sealed %s client secret could not be opened: %w", k.name, err)
	}
	// The unsealed secret rides back in the ref field, which is where the caller
	// looks for it: nothing downstream re-reads a ref it was handed, and a second
	// struct differing in one field's MEANING is how a ref reaches a vendor.
	return ConnectorApp{ClientID: app.ClientID, ClientSecretRef: string(secret), Tenant: app.Tenant}, true, nil
}

// Set stores one provider's app, sealing the secret and replacing whatever was
// there.
//
// The new secret is sealed BEFORE the ref moves and the old one is retired only
// after the move commits, so no window exists in which the recorded ref names
// nothing. A failure part-way leaves the previous app serving.
func (s *ConnectorAppStore) Set(ctx context.Context, p AppProvider, clientID, clientSecret, tenant string) error {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return err
	}
	k, err := appKindFor(p)
	if err != nil {
		return err
	}
	// Trimmed because every field is pasted from a vendor console, and a value
	// carrying a trailing newline authenticates nothing while looking exactly
	// like one that would.
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	tenant = strings.TrimSpace(tenant)
	if err := checkAppInput(k, clientID, clientSecret, tenant); err != nil {
		return err
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
		return fmt.Errorf("capture: sealing the %s client secret: %w", k.name, err)
	}
	next := ConnectorApp{ClientID: clientID, ClientSecretRef: string(ref), Tenant: tenant}
	previous, err := s.swap(ctx, ws, k, next, string(ref))
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, string(p)+" app rotated")
	return nil
}

// refuseStrandingConnections refuses a CLIENT ID change while mailboxes are
// connected under the old one.
//
// A refresh token belongs to the client that issued it. Swapping the id makes
// every stored token unrefreshable against the new one, and the vendor answers
// `invalid_client` — so the mailboxes stop syncing, one by one, at whatever hour
// their next refresh falls. Nothing in the product says why, because from the
// inside it looks like every mailbox revoking access at once.
//
// The SECRET is a different matter and stays rotatable: a new secret for the
// same client refreshes the same tokens, which is the whole point of being able
// to rotate one.
//
// Asked under the same lock as the write, so a connection made between the check
// and the commit cannot slip past it.
//
// A first configuration is not a change, and neither is re-entering the id an
// installation already holds — the operator rotating a secret sends both fields,
// and refusing them would make rotation impossible.
func refuseStrandingConnections(ctx context.Context, tx pgx.Tx, k appKind, current, next ConnectorApp) error {
	if current.ClientID == "" || current.ClientID == next.ClientID {
		return nil
	}
	// 'error' counts as live, the same set sendableConnection and SyncOnce
	// select on: an errored connection still holds the refresh token the old
	// client issued, and the sweep still tries it. Counting only 'connected'
	// would wave the change through for exactly the installation most likely to
	// be making it — one whose mailboxes have started failing — and strand them
	// with the misleading error they were already chasing.
	var connected int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM capture_connection
		 WHERE provider = ANY($1) AND status IN ('connected','error') AND archived_at IS NULL`,
		k.connectors).Scan(&connected); err != nil {
		return fmt.Errorf("capture: counting the connections a %s app change would strand: %w", k.name, err)
	}
	if connected == 0 {
		return nil
	}
	return settings.InvalidValue{
		Setting: k.entry.Key(), Code: settings.CodeInvalidValue,
		Reason: fmt.Sprintf("this installation has %d mailbox connection(s) authorized against the current "+
			"%s client id, and a refresh token belongs to the client that issued it — changing the id would "+
			"stop them syncing with no way to tell why. Disconnect them first and reconnect against the new "+
			"app, or rotate the secret and leave the id as it is", connected, k.name),
	}
}

// checkAppInput refuses what the settings validator would refuse, BEFORE a
// secret is sealed for it — the validator runs on the settings write, by which
// point the blob exists and would need retiring.
func checkAppInput(k appKind, clientID, clientSecret, tenant string) error {
	refuse := func(reason string) error {
		return settings.InvalidValue{
			Setting: k.entry.Key(), Code: settings.CodeInvalidValue, Reason: reason,
		}
	}
	// The contract declares maxLength on each, and nothing generated enforces it:
	// oapi-codegen emits no validation, and httperr.Decode caps only the whole
	// body. Without this a ~1 MiB "secret" is sealed and a ~1 MiB client id is
	// stored and handed back on every read.
	if len(clientID) > maxAppFieldBytes || len(clientSecret) > maxAppFieldBytes || len(tenant) > maxAppFieldBytes {
		return refuse(fmt.Sprintf("a %s client id, secret and tenant are each at most %d bytes", k.name, maxAppFieldBytes))
	}
	if clientID == "" || clientSecret == "" {
		return refuse(fmt.Sprintf("a %s app needs both the client id and the client secret; remove the app instead of storing half of one", k.name))
	}
	if tenant != "" && !k.directoryPinned {
		return refuse(fmt.Sprintf("a %s app is not pinned to a directory; the tenant field does nothing here", k.name))
	}
	if tenant != "" {
		if err := validateEntraTenant(tenant); err != nil {
			return refuse(err.Error())
		}
	}
	if err := k.validateClientID(k.appVendor, clientID); err != nil {
		return refuse(err.Error())
	}
	return nil
}

// Remove clears one provider's app. Removing one that was never there succeeds:
// the caller asked for a state, and that state already holds.
func (s *ConnectorAppStore) Remove(ctx context.Context, p AppProvider) error {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return err
	}
	k, err := appKindFor(p)
	if err != nil {
		return err
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return err
	}
	previous, err := s.swap(ctx, ws, k, ConnectorApp{}, "")
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, string(p)+" app removed")
	return nil
}

// swap writes the app inside a single locked transaction and returns the secret
// ref it displaced.
//
// The lock is taken before the READ for the same reason ai.ProviderKeyStore
// takes it there: the displaced ref is computed from the current value, so a
// read outside the write's transaction is a lost update — two admins rotating at
// once would each retire a blob the other still references.
func (s *ConnectorAppStore) swap(
	ctx context.Context, ws ids.WorkspaceID, k appKind, next ConnectorApp, sealed string,
) (previous string, err error) {
	// Separated from the transaction's own error so the two can be told apart
	// below; they mean different things about what committed.
	var (
		entered bool
		bodyErr error
	)
	txErr := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		entered = true
		if bodyErr = settings.LockForWrite(ctx, tx, k.entry.Key()); bodyErr != nil {
			return bodyErr
		}
		var current ConnectorApp
		if current, bodyErr = settings.GetTx(ctx, tx, k.entry); bodyErr != nil {
			return bodyErr
		}
		if bodyErr = refuseStrandingConnections(ctx, tx, k, current, next); bodyErr != nil {
			return bodyErr
		}
		previous = current.ClientSecretRef
		bodyErr = settings.SetTx(ctx, s.settings, tx, k.entry, next)
		return bodyErr
	})
	if txErr == nil {
		return previous, nil
	}
	// The closure ran and returned nil, yet the call failed: the only thing left
	// is COMMIT, and a commit that reports failure may still have committed.
	// Deleting the newly sealed secret then would leave the stored ref naming a
	// blob that is gone — an integration that cannot be repaired by re-entering
	// the secret, because the app would still read as configured. Unreferenced
	// ciphertext is inert; a dangling ref is not.
	if entered && bodyErr == nil && sealed != "" {
		s.log.WarnContext(ctx,
			"a client secret was sealed but the settings transaction did not report a clean commit; the blob is KEPT because the write may have landed, so it is either live or an orphan to reconcile",
			"vendor", k.name, "credential_ref", sealed, "err", txErr)
		return "", txErr
	}
	if sealed != "" {
		s.retire(ctx, ws, sealed, k.name+" app write refused")
	}
	return "", txErr
}

// retire deletes a secret nothing references any more, through the shared helper
// for a detached credential.
func (s *ConnectorAppStore) retire(ctx context.Context, ws ids.WorkspaceID, ref, lifecycle string) {
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
		return ids.WorkspaceID{}, errors.New("capture: an OAuth app write outside a workspace context")
	}
	return ids.From[ids.WorkspaceKind](raw), nil
}
