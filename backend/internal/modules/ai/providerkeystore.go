// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Putting a BYOK key in, taking one out, and saying which vendors have one.
//
// The vault has been where these keys LIVE since providerkeys.go landed, but
// the only way one could arrive was an environment variable the seeder picked
// up at boot. So an admin could not add a vendor without an operator, a
// restart, and a shell — and deploymentsecretseal.go's justification for
// letting the vault outrank a declaration ("a provider key has a human write
// path — the routing surface") named a path nothing had built.
//
// This is that path. It owns what a transport must not: the RBAC gate, the
// refusal of a provider this build cannot serve, sealing the bytes before they
// are recorded, and retiring the ref that was there — in that order, because a
// ref recorded before its blob exists points at nothing, and a blob left behind
// after its ref is replaced is a credential no reader can find and no operator
// can delete.

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

// ProviderKeyStatus is what a reader may learn about one vendor's credential:
// that this build can serve it, and whether a key is held. Never the key.
type ProviderKeyStatus struct {
	// Provider is the routing name — the same string a binding uses.
	Provider string
	// Configured reports whether a credential is sealed for it.
	Configured bool
	// EnvVar is the variable the key may also arrive in, so a screen can tell
	// an operator which export seeded it.
	EnvVar string
}

// ProviderKeyStore reads and changes the sealed BYOK credentials.
//
// The workspace is NOT held here: it comes off each request's context, the way
// every other tenant read resolves it. Binding one at construction would make a
// request's answer depend on which workspace happened to be resolved when the
// process booted, which is the shape of bug that only appears on the day there
// is a second one.
type ProviderKeyStore struct {
	settings *settings.Store
	vault    keyvault.Vault
	// log carries the one failure this store cannot report to its caller: a
	// superseded blob it could not delete after the write already committed.
	log *slog.Logger
}

// NewProviderKeyStore builds the store over the settings catalog and the vault.
// A nil vault is a role that seals nothing: every write refuses rather than
// recording a ref to a blob that was never written.
func NewProviderKeyStore(s *settings.Store, vault keyvault.Vault, log *slog.Logger) *ProviderKeyStore {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &ProviderKeyStore{settings: s, vault: vault, log: log}
}

// credentialWorkspace is the caller's tenant, or a refusal. A credential write outside
// a workspace has no tenant to belong to, and the vault would refuse the ref
// later anyway — so it is refused here, where the message can say why.
func credentialWorkspace(ctx context.Context) (ids.WorkspaceID, error) {
	raw, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.WorkspaceID{}, errors.New("ai: a provider key outside a workspace context")
	}
	return ids.From[ids.WorkspaceKind](raw), nil
}

// ErrVaultUnavailable is the refusal when this installation seals nothing.
// Separate from a validation error because the fix is the operator's — the
// vault root key — and not anything the caller sent.
var ErrVaultUnavailable = errors.New("this installation has no key vault configured, so a provider key cannot be stored: set the vault root key on the api and worker, then try again")

// List reports every cloud provider this build can serve and whether each holds
// a key, in the routing table's own order.
//
// It lists the providers rather than the stored refs, so a vendor with no
// credential is visible as something an admin could configure. A list of what
// happens to be stored would show an empty screen on the installation that most
// needs the screen.
func (s *ProviderKeyStore) List(ctx context.Context) ([]ProviderKeyStatus, error) {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionRead); err != nil {
		return nil, err
	}
	refs, err := settings.Get(ctx, s.settings, ProviderKeys)
	if err != nil {
		return nil, err
	}
	providers := CloudProvidersNeedingKeys()
	out := make([]ProviderKeyStatus, 0, len(providers))
	for _, provider := range providers {
		out = append(out, ProviderKeyStatus{
			Provider:   provider,
			Configured: refs[provider] != "",
			EnvVar:     KeyEnvVarFor(provider),
		})
	}
	return out, nil
}

// Set seals a key for one provider and records the ref, replacing whatever was
// there.
//
// The new blob is written BEFORE the ref is moved and the old one is retired
// only after the move commits, so no window exists in which the recorded ref
// names nothing. A failure part-way leaves the previous credential serving.
func (s *ProviderKeyStore) Set(ctx context.Context, provider, apiKey string) error {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionUpdate); err != nil {
		return err
	}
	if _, cloud := cloudKeyEnv[provider]; !cloud {
		return settings.InvalidValue{
			Setting: ProviderKeysKey, Code: settings.CodeInvalidValue,
			Reason: fmt.Sprintf("%q takes no api key", provider),
		}
	}
	// Trimmed because a pasted credential arrives with whatever whitespace the
	// clipboard carried, and a key with a trailing newline authenticates
	// nothing while looking exactly like one that would.
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return settings.InvalidValue{
			Setting: ProviderKeysKey, Code: settings.CodeInvalidValue,
			Reason: "the api key is empty — remove the credential instead of storing nothing",
		}
	}
	if s.vault == nil {
		return ErrVaultUnavailable
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return err
	}
	ref, err := s.vault.Put(ctx, ws, []byte(apiKey))
	if err != nil {
		return fmt.Errorf("ai: sealing the %s key: %w", provider, err)
	}
	previous, err := s.swapRef(ctx, ws, provider, string(ref))
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, "provider key rotated")
	return nil
}

// swapRef moves one provider's ref inside a single locked transaction and
// returns the ref it displaced.
//
// The value is a provider→ref MAP, so read-modify-write is the whole operation
// and doing the read outside the write's transaction is a lost update, not a
// conflict: two admins keying two different vendors both read the same map, and
// the second write drops the first's provider entirely. The lock is taken before
// the read for that reason — SetTx takes the same one, but by then both readers
// already hold the same snapshot.
func (s *ProviderKeyStore) swapRef(ctx context.Context, ws ids.WorkspaceID, provider, ref string) (previous string, err error) {
	// Three states, not two. `entered` records that the closure ran at all,
	// because WriteTx also fails BEFORE it — an unresolved workspace, a failed
	// Begin, a failed GUC bind — and every one of those provably did not commit.
	// Treating them as ambiguous would keep a blob that is definitionally an
	// orphan and send an operator to reconcile it.
	var (
		entered bool
		bodyErr error
	)
	txErr := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		entered = true
		if bodyErr = settings.LockForWrite(ctx, tx, ProviderKeysKey); bodyErr != nil {
			return bodyErr
		}
		var refs map[string]string
		if refs, bodyErr = settings.GetTx(ctx, tx, ProviderKeys); bodyErr != nil {
			return bodyErr
		}
		previous = refs[provider]
		next := copyRefs(refs)
		if ref == "" {
			delete(next, provider)
		} else {
			next[provider] = ref
		}
		bodyErr = settings.SetTx(ctx, s.settings, tx, ProviderKeys, next)
		return bodyErr
	})
	if txErr == nil {
		return previous, nil
	}
	// A blob was sealed for a write that may not have landed. Whether it is safe
	// to drop turns on WHICH half failed, and getting that backwards is the
	// worse outcome in one direction only.
	//
	// The closure ran and returned nil, yet the call failed: the only thing left
	// is COMMIT, and a commit that reports failure may still have committed.
	// Deleting then would leave the stored ref naming a blob that is gone — an
	// AI lane that cannot be repaired by re-entering the key, because the ref
	// would still read as configured. So the blob stays: unreferenced ciphertext
	// is inert, and an orphan is recoverable where a dangling ref is not.
	//
	// Every other shape — the body refused, or it never ran — certainly rolled
	// back, so nothing can reference the new blob and dropping it is safe.
	if entered && bodyErr == nil && ref != "" {
		s.log.WarnContext(ctx,
			"a provider key was sealed but the settings transaction did not report a clean commit; the blob is KEPT because the write may have landed, so it is either live or an orphan to reconcile",
			// The refs whole, not masked: this line exists so an operator can
			// FIND the blob to reconcile, and a ref with its token elided names
			// nothing they can act on. Same choice keyvault.DeleteDetached makes
			// for the same reason, and a test there holds it.
			"provider", provider, "credential_ref", ref,
			"superseded_ref", previous, "err", txErr)
		return "", txErr
	}
	if ref != "" {
		s.retire(ctx, ws, ref, "provider key write refused")
	}
	return "", txErr
}

// Remove retires a provider's credential. Removing one that was never there
// succeeds: the caller asked for a state, and that state already holds.
func (s *ProviderKeyStore) Remove(ctx context.Context, provider string) error {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionUpdate); err != nil {
		return err
	}
	ws, err := credentialWorkspace(ctx)
	if err != nil {
		return err
	}
	// Through the same locked swap, with an empty ref meaning "drop the entry":
	// a Remove read outside the write would lose a concurrent Set on another
	// provider exactly as a Set would.
	previous, err := s.swapRef(ctx, ws, provider, "")
	if err != nil {
		return err
	}
	s.retire(ctx, ws, previous, "provider key removed")
	return nil
}

// retire deletes a blob nothing references any more, through the shared
// helper for a detached credential (keyvault.DeleteDetached).
//
// Not a hand-rolled delete, because that helper already answers the three
// questions this call raises and answers them the same way for every custodian
// in the tree: the caller's write has committed so the failure must not be
// reported as theirs; the blob is inert and its REF is safe to log, so a
// failure is logged at ERROR for cleanup rather than swallowed; and the delete
// is detached from the request's cancellation under its own deadline, so a
// client that hangs up the instant it has its answer does not strand the blob.
func (s *ProviderKeyStore) retire(ctx context.Context, ws ids.WorkspaceID, ref, lifecycle string) {
	keyvault.DeleteDetached(ctx, s.vault, s.log, ws.UUID, keyvault.Ref(ref), lifecycle)
}

// copyRefs takes a copy before mutating, so a failed write leaves the map the
// caller read unchanged.
func copyRefs(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}
