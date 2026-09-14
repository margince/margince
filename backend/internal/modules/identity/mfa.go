// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Multi-factor authentication: a member enrols a TOTP authenticator and holds a
// set of one-time recovery codes for when it is lost. The enrolment surface is
// self-service — the caller acts on their own row and nobody else's, the same
// self-scoping the session list uses — while the verify path is called by the
// login flow for a member the password has already identified.
//
// The secret is SEALED, not hashed: a code can only be checked by recomputing it
// from the secret, so the material must be recoverable. It lives in the key
// vault, and this table holds only the ref.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	recoveryCodeCount = 10
	recoveryCodeBytes = 10 // ~16 base64url chars: enough entropy, short enough to type.
)

var (
	errMFAAlreadyEnrolled = errors.New("identity: multi-factor authentication is already enrolled")
	errMFANotEnrolled     = errors.New("identity: no multi-factor enrolment to confirm")
	errBadMFACode         = errors.New("identity: the code is not valid")
)

// MFAStatus is what a member sees about their own second factor.
type MFAStatus struct {
	Enrolled          bool
	Confirmed         bool
	RecoveryCodesLeft int
}

// WithVault injects the seal the MFA methods store TOTP secrets in. Unset leaves
// MFA unavailable — the enrolment methods refuse rather than store a secret in
// the clear.
func (s *Service) WithVault(v keyvault.Vault) *Service {
	s.vault = v
	return s
}

// WithMFAVault wires the seal into the handlers' service, so the composition
// root can turn MFA on where a vault exists. A deployment with no vault serves
// the enrolment routes as unavailable rather than storing a secret in the clear.
// Named distinctly from the capture handlers' own WithVault, so neither shadows
// the other on the embedding server.
//
// A nil service is a no-op: a Server assembled without the auth handlers (the
// capture-wiring tests build one) still runs the vault option, and it has no
// identity service to seal into.
func (h Handlers) WithMFAVault(v keyvault.Vault) Handlers {
	if h.svc != nil {
		h.svc.WithVault(v)
	}
	return h
}

// StartTOTPEnrolment mints a fresh secret for the caller, seals it, and records a
// PENDING enrolment. Returns the raw secret once, for the authenticator app; it
// is never shown again. A caller who is already CONFIRMED must disable first —
// silently swapping the factor would leave a member trusting an authenticator
// that no longer guards them.
//
// The seal happens before the transaction (the row stores the ref), so every
// exit that does not commit the fresh ref must destroy the blob it references:
// a refused or failed enrolment would otherwise strand a sealed secret nothing
// can ever reach again. A RESTARTED pending enrolment supersedes the previous
// ref the same way, destroyed only after the commit that unreferenced it.
func (s *Service) StartTOTPEnrolment(ctx context.Context) (string, error) {
	human, wsID, err := s.selfWorkspace(ctx)
	if err != nil {
		return "", err
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return "", err
	}
	ref, err := s.vault.Put(ctx, wsID, []byte(secret))
	if err != nil {
		return "", fmt.Errorf("identity: seal totp secret: %w", err)
	}
	var superseded string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		superseded = ""
		prevRef, confirmedAt, _, lockErr := lockEnrolment(ctx, tx, human)
		switch {
		case errors.Is(lockErr, errMFANotEnrolled):
			// First enrolment: nothing to supersede.
		case lockErr != nil:
			return lockErr
		case confirmedAt != nil:
			return errMFAAlreadyEnrolled
		default:
			superseded = prevRef
		}
		// last_used_step resets with the secret: a fresh secret opens a fresh
		// step namespace, and a stale high-water mark would refuse its first code.
		_, err := tx.Exec(ctx,
			`INSERT INTO user_mfa (user_id, secret_ref) VALUES ($1, $2)
			 ON CONFLICT (user_id) DO UPDATE
			   SET secret_ref = EXCLUDED.secret_ref, confirmed_at = NULL,
			       last_used_step = 0, created_at = now()`,
			human, string(ref))
		return err
	})
	if err != nil {
		// The row never took the fresh ref, so the blob it names is orphaned the
		// moment this returns; post-commit-style detached delete, because the
		// refusal (already-enrolled) is the authoritative outcome either way.
		keyvault.DeleteDetached(ctx, s.vault, slog.Default(), wsID.UUID, ref, "totp-enrolment-refused")
		return "", err
	}
	if superseded != "" {
		keyvault.DeleteDetached(ctx, s.vault, slog.Default(), wsID.UUID, keyvault.Ref(superseded), "totp-enrolment-restarted")
	}
	return secret, nil
}

// ConfirmTOTP verifies a code against the pending secret and, on success,
// activates the factor and issues one-time recovery codes — returned once, the
// only moment they are shown. A wrong code leaves the enrolment pending.
//
// Success also ends every OTHER session the member holds, sparing only the one
// whose token hashes to keepTokenHash (empty keeps none). MustEnrolMFA is
// account-wide, so a session opened before the factor existed would gain full
// access the instant any session confirms — including one a hijacker was
// sitting on, which the member enrolling a factor is often reacting to. The
// accepted step is recorded so the confirmation code cannot be replayed at the
// first login.
func (s *Service) ConfirmTOTP(ctx context.Context, code, keepTokenHash string) ([]string, error) {
	human, wsID, err := s.selfWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	var recovery []string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		secret, confirmedAt, lastUsedStep, err := lockEnrolment(ctx, tx, human)
		if err != nil {
			return err
		}
		if confirmedAt != nil {
			return errMFAAlreadyEnrolled
		}
		plain, err := s.vault.GetOn(ctx, tx, wsID, keyvault.Ref(secret))
		if err != nil {
			return fmt.Errorf("identity: open totp secret: %w", err)
		}
		ok, step, err := verifyTOTPCode(string(plain), code, s.now())
		if err != nil {
			return err
		}
		if !ok || step <= lastUsedStep {
			return errBadMFACode
		}
		if _, err := tx.Exec(ctx,
			`UPDATE user_mfa SET confirmed_at = now(), last_used_step = $2 WHERE user_id = $1`,
			human, step); err != nil {
			return err
		}
		recovery, err = issueRecoveryCodes(ctx, tx, human)
		if err != nil {
			return err
		}
		if err := revokeOtherSessions(ctx, tx, human, keepTokenHash); err != nil {
			return err
		}
		return logAuthEvent(ctx, tx, human, "mfa_enrolled", "authenticator confirmed and recovery codes issued")
	})
	if err != nil {
		return nil, err
	}
	return recovery, nil
}

// revokeOtherSessions ends every live session of the member except the one
// carrying keepTokenHash, and records why. Run in the confirming transaction so
// the factor becoming live and the pre-factor sessions dying are one fact — a
// crash between them would leave a session the new factor never vouched for.
func revokeOtherSessions(ctx context.Context, tx pgx.Tx, userID ids.UserID, keepTokenHash string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE session SET revoked_at = now()
		  WHERE user_id = $1 AND revoked_at IS NULL AND token_hash <> $2`,
		userID, keepTokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return logAuthEvent(ctx, tx, userID, "mfa_sessions_revoked",
		"other sessions ended because a second factor was established")
}

// MFAEnrolment reports the caller's own second-factor state.
func (s *Service) MFAEnrolment(ctx context.Context) (MFAStatus, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return MFAStatus{}, err
	}
	userID := ids.From[ids.UserKind](human)
	var status MFAStatus
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var confirmedAt *time.Time
		scanErr := tx.QueryRow(ctx,
			`SELECT confirmed_at FROM user_mfa WHERE user_id = $1`, userID).Scan(&confirmedAt)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		status.Enrolled = true
		status.Confirmed = confirmedAt != nil
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM mfa_recovery_code WHERE user_id = $1 AND used_at IS NULL`,
			userID).Scan(&status.RecoveryCodesLeft)
	})
	if err != nil {
		return MFAStatus{}, fmt.Errorf("identity: read mfa enrolment: %w", err)
	}
	return status, nil
}

// DisableMFA removes the caller's own factor — the enrolment, its recovery codes
// (they cascade with the user_mfa row), and the sealed secret. Idempotent:
// disabling when nothing is enrolled is a no-op.
//
// Removing a CONFIRMED factor is a step-up operation: code must be a current
// authenticator code or an unused recovery code, verified in the same
// transaction that deletes the row. A session alone must not suffice — a
// hijacked session could otherwise strip the factor and enrol its own. A
// pending enrolment needs no code: it guards nothing yet, and restarting an
// enrolment already replaces it freely.
func (s *Service) DisableMFA(ctx context.Context, code string) error {
	human, wsID, err := s.selfWorkspace(ctx)
	if err != nil {
		return err
	}
	var ref string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, confirmedAt, _, lockErr := lockEnrolment(ctx, tx, human)
		if errors.Is(lockErr, errMFANotEnrolled) {
			return nil
		}
		if lockErr != nil {
			return lockErr
		}
		if confirmedAt != nil {
			verified, verifyErr := s.verifyMFA(ctx, tx, wsID, human, code)
			if verifyErr != nil {
				return verifyErr
			}
			if !verified {
				return errBadMFACode
			}
		}
		if scanErr := tx.QueryRow(ctx,
			`DELETE FROM user_mfa WHERE user_id = $1 RETURNING secret_ref`, human).Scan(&ref); scanErr != nil {
			return scanErr
		}
		return logAuthEvent(ctx, tx, human, "mfa_disabled", "authenticator and recovery codes removed")
	})
	if err != nil {
		return err
	}
	if ref != "" {
		// After the commit the row is gone and nothing references the blob; the
		// disable the member asked for already happened, so the erase runs
		// detached and can only ever log — failing the request here would report
		// a disable that DID happen as one that did not.
		keyvault.DeleteDetached(ctx, s.vault, slog.Default(), wsID.UUID, keyvault.Ref(ref), "mfa-disabled")
	}
	return nil
}

// selfWorkspace resolves the caller from the principal and the installation's
// workspace together — the pair every MFA write needs: whose row, and which
// vault scope the secret seals under.
func (s *Service) selfWorkspace(ctx context.Context) (ids.UserID, ids.WorkspaceID, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return ids.UserID{}, ids.WorkspaceID{}, err
	}
	wsID, err := s.InstallationWorkspace(ctx)
	if err != nil {
		return ids.UserID{}, ids.WorkspaceID{}, err
	}
	if s.vault == nil {
		return ids.UserID{}, ids.WorkspaceID{}, fmt.Errorf("identity: no secret vault is configured for multi-factor authentication")
	}
	return ids.From[ids.UserKind](human), ids.From[ids.WorkspaceKind](wsID.UUID), nil
}

// lockEnrolment reads a member's enrolment under a row lock, so a concurrent
// confirm and disable cannot interleave — and so two verifications of one code
// serialize on the last_used_step high-water mark instead of both accepting it.
func lockEnrolment(ctx context.Context, tx pgx.Tx, userID ids.UserID) (secretRef string, confirmedAt *time.Time, lastUsedStep int64, err error) {
	scanErr := tx.QueryRow(ctx,
		`SELECT secret_ref, confirmed_at, last_used_step FROM user_mfa WHERE user_id = $1 FOR UPDATE`,
		userID).Scan(&secretRef, &confirmedAt, &lastUsedStep)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		return "", nil, 0, errMFANotEnrolled
	}
	if scanErr != nil {
		return "", nil, 0, scanErr
	}
	return secretRef, confirmedAt, lastUsedStep, nil
}

// issueRecoveryCodes replaces any existing codes with a fresh set, returning the
// raw codes to show once. Each is stored only as a hash, the session-token
// posture: the table never holds anything presentable.
func issueRecoveryCodes(ctx context.Context, tx pgx.Tx, userID ids.UserID) ([]string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM mfa_recovery_code WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	raw := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		code, err := randomTokenOfLength(recoveryCodeBytes)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO mfa_recovery_code (user_id, code_hash) VALUES ($1, $2)`,
			userID, hashToken(code)); err != nil {
			return nil, err
		}
		raw = append(raw, code)
	}
	return raw, nil
}

// hasConfirmedMFA reports whether the member has an active second factor, the
// question the login flow asks to decide whether a password alone completes.
func hasConfirmedMFA(ctx context.Context, tx pgx.Tx, userID ids.UserID) (bool, error) {
	var confirmed bool
	err := tx.QueryRow(ctx,
		`SELECT confirmed_at IS NOT NULL FROM user_mfa WHERE user_id = $1`, userID).Scan(&confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return confirmed, nil
}

// verifyMFA checks a second factor for a member the login flow has already
// identified: a live authenticator code, or an unused recovery code (spent on
// use). Both branches are WRITES — an accepted authenticator code records its
// time-step so the same code is refused a second time inside the skew window,
// and a recovery code stamps itself spent — so this runs in the caller's
// transaction, under the enrolment row lock.
func (s *Service) verifyMFA(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, userID ids.UserID, code string) (bool, error) {
	secretRef, confirmedAt, lastUsedStep, err := lockEnrolment(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if confirmedAt == nil {
		return false, errMFANotEnrolled
	}
	plain, err := s.vault.GetOn(ctx, tx, wsID, keyvault.Ref(secretRef))
	if err != nil {
		return false, fmt.Errorf("identity: open totp secret: %w", err)
	}
	ok, step, err := verifyTOTPCode(string(plain), code, s.now())
	if err != nil {
		return false, err
	}
	if ok {
		// A code from a step already honoured is a replay, refused as neutrally
		// as a wrong code — it IS the right code, in the wrong hands or twice.
		if step <= lastUsedStep {
			return false, nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE user_mfa SET last_used_step = $2 WHERE user_id = $1`, userID, step); err != nil {
			return false, err
		}
		return true, nil
	}
	return consumeRecoveryCode(ctx, tx, userID, code)
}

// consumeRecoveryCode spends one unused recovery code matching the presented
// value, by its hash. It marks exactly the row it matched, so a code works once
// and never again.
func consumeRecoveryCode(ctx context.Context, tx pgx.Tx, userID ids.UserID, code string) (bool, error) {
	tag, err := tx.Exec(ctx,
		`UPDATE mfa_recovery_code SET used_at = now()
		  WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		userID, hashToken(code))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
