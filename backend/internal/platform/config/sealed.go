// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// Sealer keeps a secret setting's value out of its row. Compose builds one over
// the key vault; this package cannot import the vault, which reads its own root
// key through a Config.
type Sealer struct {
	Seal    func(ctx context.Context, plaintext string) (ref string, err error)
	Resolve func(ctx context.Context, ref string) (plaintext string, err error)
	// Retire deletes a sealed value no row names any more. It runs after the
	// write commits, so it reports to a log and never to the caller.
	Retire func(ctx context.Context, ref string)
}

// WithSealer returns a copy that seals and opens SecretReference settings
// through s.
func (c *Config) WithSealer(s Sealer) *Config {
	next := *c
	next.sealer = &s
	return &next
}

// Seals reports whether this Config can seal and open a secret setting.
func (c *Config) Seals() bool { return c != nil && c.sealer != nil }

func (c *Config) sealerFor(key string) (*Sealer, error) {
	if !c.Seals() {
		return nil, fmt.Errorf("config: %s is sealed in the key vault and this Config has no vault; build it WithSealer", key)
	}
	return c.sealer, nil
}

// errEmptyRef refuses a row that claims a sealed value and names none.
var errEmptyRef = errors.New("a sealed value's reference cannot be empty; remove the setting instead")

func sealedEntry(key string, st Store) *settings.Entry[string] {
	row := settings.Define(key, st.RBACObject, st.AuditVerb, "", func(ref string) error {
		if ref == "" {
			return errEmptyRef
		}
		return nil
	}).AsSecretReference()
	if st.Freeze != nil {
		row = row.WithFreeze(st.Freeze)
	}
	if st.SurvivesDataReset {
		row = row.AsInstallationIdentity()
	}
	return row
}

// openSealed reads the value a ref names. Neither the ref nor the value
// reaches the error.
func openSealed[T any](ctx context.Context, c *Config, s *Setting[T], ref string) (T, error) {
	var zero T
	sealer, err := c.sealerFor(s.Key())
	if err != nil {
		return zero, err
	}
	text, err := sealer.Resolve(ctx, ref)
	if err != nil {
		return zero, fmt.Errorf("config: opening the sealed %s: %w", s.Key(), err)
	}
	v, err := s.spec.Type.Parse(text)
	if err != nil {
		return zero, fmt.Errorf("config: the sealed %s %w", s.Key(), errInvalidSecret)
	}
	return v, nil
}

// storedSealed is the row's value, opened: zero and false where no row exists.
func storedSealed[T any](ctx context.Context, c *Config, db *settings.Store, s *Setting[T]) (T, bool, error) {
	var zero T
	ref, stored, err := settings.GetStored(ctx, db, s.sealed)
	if err != nil || !stored {
		return zero, false, err
	}
	v, err := openSealed(ctx, c, s, ref)
	return v, true, err
}

// saveSealed validates the value before it seals it, so a refused value leaves
// no blob behind. It then points the row at the new blob and retires the old.
func saveSealed[T any](ctx context.Context, c *Config, s *Setting[T], v T) error {
	if err := s.checkTyped(v); err != nil {
		return settings.InvalidValue{Setting: s.Key(), Code: settings.CodeInvalidValue, Reason: err.Error()}
	}
	db, err := c.database(s.Key())
	if err != nil {
		return err
	}
	sealer, err := c.sealerFor(s.Key())
	if err != nil {
		return err
	}
	ref, err := sealer.Seal(ctx, s.spec.Type.Format(v))
	if err != nil {
		return fmt.Errorf("config: sealing %s: %w", s.Key(), err)
	}
	return swapSealed(ctx, db, sealer, s.sealed, ref)
}

// RemoveSecret deletes a sealed setting's row and then its blob. Removing one
// that was never stored succeeds. A boot source that sets it refuses the
// removal, like any other change.
func RemoveSecret[T any](ctx context.Context, c *Config, s *Setting[T]) error {
	if err := requireSealed(s); err != nil {
		return err
	}
	if err := settings.RequireWrite(ctx, s.sealed); err != nil {
		return err
	}
	if origin, locked := c.Locked(s.Key()); locked {
		return LockedError{Key: s.Key(), Origin: origin}
	}
	db, err := c.database(s.Key())
	if err != nil {
		return err
	}
	sealer, err := c.sealerFor(s.Key())
	if err != nil {
		return err
	}
	return swapSealed(ctx, db, sealer, s.sealed, "")
}

// swapSealed points the row at next, or deletes it for "". The write lock is
// taken before the read, so two writers cannot both retire one blob.
//
// A commit that reports failure after the body succeeded may still have
// landed. The new blob is then kept: an orphan is inert, while a row naming a
// deleted blob cannot be repaired by saving again.
func swapSealed(ctx context.Context, db *settings.Store, sealer *Sealer, row *settings.Entry[string], next string) error {
	var previous string
	var bodyErr error
	entered := false
	txErr := db.WriteTx(ctx, func(tx pgx.Tx) error {
		entered = true
		previous, bodyErr = swapSealedTx(ctx, db, tx, row, next)
		return bodyErr
	})
	if txErr != nil {
		if entered && bodyErr == nil && next != "" {
			return fmt.Errorf("config: %s was sealed but its write did not report a clean commit, so the sealed copy is kept: %w", row.Key(), txErr)
		}
		if next != "" {
			sealer.Retire(ctx, next)
		}
		return txErr
	}
	if previous != "" && previous != next {
		sealer.Retire(ctx, previous)
	}
	return nil
}

func swapSealedTx(ctx context.Context, db *settings.Store, tx pgx.Tx, row *settings.Entry[string], next string) (string, error) {
	if err := settings.LockForWrite(ctx, tx, row.Key()); err != nil {
		return "", err
	}
	previous, err := settings.GetTx(ctx, tx, row)
	if err != nil {
		return "", err
	}
	if next == "" {
		_, err = settings.DeleteTx(ctx, db, tx, row)
		return previous, err
	}
	return previous, settings.SetTx(ctx, db, tx, row, next)
}

// SealedRef is the vault ref a secret's row holds, unopened, whether or not a
// boot source sets the secret. It is what a reader that must notice a rotation
// compares, without ever holding the secret.
func SealedRef[T any](ctx context.Context, c *Config, s *Setting[T]) (ref string, stored bool, err error) {
	if err := requireSealed(s); err != nil {
		return "", false, err
	}
	if err := settings.RequireRead(ctx, s.sealed); err != nil {
		return "", false, err
	}
	db, err := c.database(s.Key())
	if err != nil {
		return "", false, err
	}
	return settings.GetStored(ctx, db, s.sealed)
}

// requireSealed refuses a setting whose row holds its value rather than a ref.
func requireSealed[T any](s *Setting[T]) error {
	if err := requireStored(s); err != nil {
		return err
	}
	if s.sealed == nil {
		return fmt.Errorf("config: %s is not a SecretReference setting", s.Key())
	}
	return nil
}

// requirePlain refuses a sealed setting on a path that reads or writes the row
// as the value itself.
func requirePlain[T any](s *Setting[T]) error {
	if err := requireStored(s); err != nil {
		return err
	}
	if s.sealed != nil {
		return fmt.Errorf("config: %s is sealed in the key vault; read it with StoredVal and write it with Save", s.Key())
	}
	return nil
}
