// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The Google directory connection: the sealed credential an admin stores, its
// status, and the lookup a Google sign-in makes with it. The lookup only feeds
// the groups the claim path already uses (grouprolesync.go), so a failed one
// grants nothing and takes nothing away.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// directoryProvider is the one sign-in provider whose token carries no groups.
const directoryProvider = "google"

const (
	// directoryLogAction is the system_log action of every directory check.
	// The status read takes the newest row, and an operator reads the rest.
	directoryLogAction = "google_directory"
	directoryOK        = "ok"
	directoryFailed    = "failed"

	directoryActor = "system:google_directory"
)

// GoogleDirectoryCredential is the sealed bundle of the service-account key file
// and the admin address it acts for. Write-only: nothing returns it, and the row
// holds the vault reference.
var GoogleDirectoryCredential = config.DefineStored(config.Spec[string]{
	Key: "identity.google_directory_credential", Env: "MARGINCE_IDENTITY_GOOGLE_DIRECTORY_CREDENTIAL",
	Type: config.String, Secret: true,
	Doc: "The service-account key and admin address that read a Google member's groups at sign-in, as JSON with service_account_key and admin_subject.",
}, config.Store{
	RBACObject: authenticationPolicyObject, AuditVerb: installationAuditVerb, SecretReference: true, SurvivesDataReset: true,
})

// ErrDirectoryUnavailable is the refusal when this installation seals nothing.
var ErrDirectoryUnavailable = errors.New("this installation has no key vault configured, so a Google directory credential cannot be stored: set the vault root key on the api and worker, then try again")

type googleDirectory struct {
	cfg    *config.Config
	reader *GoogleGroupsReader
}

// WithGoogleDirectory turns the Google group lookup on. cfg must seal secret
// settings; a service without it never looks anything up.
func (s *Service) WithGoogleDirectory(cfg *config.Config) *Service {
	s.directory = &googleDirectory{cfg: cfg, reader: NewGoogleGroupsReader()}
	return s
}

// WithGoogleDirectory wires the directory connection into the handlers' service.
// A nil service is a no-op, like WithMFAVault.
func (h Handlers) WithGoogleDirectory(cfg *config.Config) Handlers {
	if h.svc != nil {
		h.svc.WithGoogleDirectory(cfg)
	}
	return h
}

// DirectoryInput is what an admin sends to connect the directory.
type DirectoryInput struct {
	ServiceAccountJSON string
	AdminSubject       string
}

// DirectoryStatus is what a reader may learn about the connection. Never the key.
type DirectoryStatus struct {
	Configured    bool
	LastCheckedAt *time.Time
	// LastResult is "ok" or "failed", empty before any check.
	LastResult string
	// LastReason names the failure class when LastResult is "failed".
	LastReason string
}

// GoogleDirectoryStatus reports whether a credential is held and how the last
// check went.
func (s *Service) GoogleDirectoryStatus(ctx context.Context) (DirectoryStatus, error) {
	if err := auth.Require(ctx, authenticationPolicyObject, principal.ActionRead); err != nil {
		return DirectoryStatus{}, err
	}
	if s.directory == nil {
		return DirectoryStatus{}, nil
	}
	_, locked := s.directory.cfg.Locked(GoogleDirectoryCredential.Key())
	_, stored, err := config.SealedRef(ctx, s.directory.cfg, GoogleDirectoryCredential)
	if err != nil {
		return DirectoryStatus{}, err
	}
	status := DirectoryStatus{Configured: locked || stored}
	if !status.Configured {
		return status, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var at time.Time
		var result, reason *string
		scanErr := tx.QueryRow(ctx,
			`SELECT occurred_at, detail->>'outcome', detail->>'reason' FROM system_log
			 WHERE action = $1 ORDER BY occurred_at DESC LIMIT 1`, directoryLogAction).Scan(&at, &result, &reason)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		status.LastCheckedAt = &at
		status.LastResult, status.LastReason = deref(result), deref(reason)
		return nil
	})
	return status, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SetGoogleDirectory checks the credential against Google and, only if Google
// accepts it, seals it, replacing any earlier one. The check reads the admin's
// own groups, which proves the key, the delegation, the scope and the admin
// address in one call.
func (s *Service) SetGoogleDirectory(ctx context.Context, in DirectoryInput) (DirectoryStatus, error) {
	if err := auth.Require(ctx, authenticationPolicyObject, principal.ActionUpdate); err != nil {
		return DirectoryStatus{}, err
	}
	if s.directory == nil || !s.directory.cfg.Seals() {
		return DirectoryStatus{}, ErrDirectoryUnavailable
	}
	if _, err := config.LockedChange(s.directory.cfg, GoogleDirectoryCredential, func(string) bool { return false }); err != nil {
		return DirectoryStatus{}, err
	}
	cred := directoryCredential{
		ServiceAccountKey: strings.TrimSpace(in.ServiceAccountJSON), AdminSubject: strings.TrimSpace(in.AdminSubject),
	}
	if err := cred.validate(); err != nil {
		return DirectoryStatus{}, directoryRefused(err.Error())
	}
	if _, err := s.directory.reader.Groups(ctx, cred, cred.AdminSubject); err != nil {
		reason := directoryReason(err)
		s.recordDirectoryCheck(ctx, directoryFailed, reason, true)
		return DirectoryStatus{}, directoryRefused(directoryAdvice[reason])
	}
	sealed, err := json.Marshal(cred)
	if err != nil {
		return DirectoryStatus{}, fmt.Errorf("identity: encode the directory credential: %w", err)
	}
	if err := config.Save(ctx, s.directory.cfg, GoogleDirectoryCredential, string(sealed)); err != nil {
		return DirectoryStatus{}, err
	}
	s.recordDirectoryCheck(ctx, directoryOK, "", true)
	return s.GoogleDirectoryStatus(ctx)
}

// RemoveGoogleDirectory deletes the credential and its sealed copy. Removing one
// that was never stored succeeds.
func (s *Service) RemoveGoogleDirectory(ctx context.Context) error {
	if err := auth.Require(ctx, authenticationPolicyObject, principal.ActionUpdate); err != nil {
		return err
	}
	if s.directory == nil || !s.directory.cfg.Seals() {
		return ErrDirectoryUnavailable
	}
	if _, err := config.LockedChange(s.directory.cfg, GoogleDirectoryCredential, func(string) bool { return false }); err != nil {
		return err
	}
	return config.RemoveSecret(ctx, s.directory.cfg, GoogleDirectoryCredential)
}

func directoryRefused(reason string) error {
	return settings.InvalidValue{Setting: GoogleDirectoryCredential.Key(), Code: settings.CodeInvalidValue, Reason: reason}
}

// directoryAdvice says what to fix for each failure class, in words an admin can
// act on and with nothing of Google's own text in them.
var directoryAdvice = map[DirectoryFailure]string{
	DirectoryKeyInvalid:        "the service-account key file could not be used to sign",
	DirectoryDelegationRefused: "Google refused the delegation: grant the service account's client ID domain-wide delegation for the directory group read-only scope, and check the admin address",
	DirectoryPermissionDenied:  "Google refused the group read: the admin address must belong to a Workspace admin who may read groups",
	DirectoryNotFound:          "Google does not know the admin address",
	DirectoryTimeout:           "Google did not answer in time: try again",
	DirectoryUnavailable:       "Google could not be reached: try again",
	DirectoryBadAnswer:         "Google answered in a form this server cannot read",
}

// groupsFor is the groups a federated sign-in grants from. A token that carries
// groups wins and no lookup runs. A Google member's groups otherwise come from
// the directory, but only when a lookup could change something. That means the
// member is already invited, the map has an entry and a credential is stored.
// It runs before the login transaction opens, so no network call waits inside
// the transaction that holds the authorization lock.
//
// It never fails the sign-in. A lookup that cannot answer grants nothing, as a
// groupless token does. The map never revokes, so nothing is taken away. The
// reason goes to the operator (recordDirectoryCheck).
func (s *Service) groupsFor(ctx context.Context, provider string, claims OIDCClaims) []string {
	if len(claims.Groups) > 0 || s.directory == nil || provider != directoryProvider {
		return claims.Groups
	}
	if !s.directoryLookupWanted(ctx, provider, claims) {
		return nil
	}
	cred, ok := s.directoryCredential(ctx)
	if !ok {
		return nil
	}
	groups, err := s.directory.reader.Groups(ctx, cred, claims.Email)
	if err != nil {
		s.recordDirectoryCheck(ctx, directoryFailed, directoryReason(err), false)
		return nil
	}
	s.recordDirectoryCheck(ctx, directoryOK, "", false)
	return groups
}

// directoryLookupWanted answers whether this sign-in could use a lookup. The
// member resolution here is read-only and runs again in the login transaction,
// which refuses an address nobody invited as before.
func (s *Service) directoryLookupWanted(ctx context.Context, provider string, claims OIDCClaims) bool {
	if s.groupRoleMap == nil {
		return false
	}
	wanted := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, _, err := s.resolveFederatedUser(ctx, tx, provider, claims.Subject, claims.Email); err != nil {
			if errors.Is(err, ErrFederatedSignInRefused) {
				return nil
			}
			return err
		}
		roleMap, err := s.groupRoleMap(ctx, tx)
		wanted = len(roleMap) > 0
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "checking whether a Google directory lookup is needed", "err", err)
		return false
	}
	return wanted
}

// directoryCredential opens the stored credential, read as the installation: the
// anonymous sign-in has no workspace bound yet. ok is false when none is stored.
func (s *Service) directoryCredential(ctx context.Context) (directoryCredential, bool) {
	ws, err := s.InstallationWorkspace(ctx)
	if err != nil {
		slog.WarnContext(ctx, "resolving the installation for the Google directory", "err", err)
		return directoryCredential{}, false
	}
	readCtx := principal.WithWorkspaceID(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: directoryActor,
	}), ws.UUID)
	sealed, err := config.StoredVal(readCtx, s.directory.cfg, GoogleDirectoryCredential)
	if err != nil {
		s.recordDirectoryCheck(ctx, directoryFailed, DirectoryKeyInvalid, false)
		return directoryCredential{}, false
	}
	if sealed == "" {
		return directoryCredential{}, false
	}
	cred, err := parseDirectoryCredential(sealed)
	if err != nil {
		s.recordDirectoryCheck(ctx, directoryFailed, DirectoryKeyInvalid, false)
		return directoryCredential{}, false
	}
	return cred, true
}

// recordDirectoryCheck writes one system_log row for the status read and the
// operator. A failure always writes one. A success writes one only when it ends
// a run of failures or force is set, for a credential just stored. A busy
// directory therefore adds no row per sign-in. The reason is a closed class with
// no key material.
func (s *Service) recordDirectoryCheck(ctx context.Context, outcome string, reason DirectoryFailure, force bool) {
	if outcome == directoryFailed {
		slog.WarnContext(ctx, "a Google directory check failed; a failed sign-in lookup grants nothing", "reason", reason)
	}
	writeCtx, cancel := detachedForFailure(ctx)
	defer cancel()
	writeCtx = principal.WithActor(writeCtx, principal.Principal{Type: principal.PrincipalSystem, ID: directoryActor})
	if err := s.db.Tx(writeCtx, func(tx pgx.Tx) error {
		if outcome == directoryOK && !force {
			var newest *string
			err := tx.QueryRow(writeCtx,
				`SELECT detail->>'outcome' FROM system_log WHERE action = $1 ORDER BY occurred_at DESC LIMIT 1`,
				directoryLogAction).Scan(&newest)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if newest != nil && *newest == directoryOK {
				return nil
			}
		}
		_, err := storekit.LogSystem(writeCtx, tx, directoryLogAction,
			map[string]any{"outcome": outcome, "reason": string(reason)})
		return err
	}); err != nil {
		slog.ErrorContext(ctx, "recording a Google directory check", "err", err)
	}
}
