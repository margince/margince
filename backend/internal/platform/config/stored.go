// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// LockedError refuses a write to a stored setting that a boot source sets. The
// row would be stored and never read, so the save would only look like it worked.
type LockedError struct {
	Key    string
	Origin Origin
}

func (e LockedError) Error() string { return e.Key + " is set by " + originText(e.Origin) }

// Is makes a lock a conflict to errors.Is, which the HTTP layer maps.
func (e LockedError) Is(target error) bool { return target == apperrors.ErrConflict }

// WithDatabase returns a copy that reads and writes stored settings through st,
// so the boot view stays usable before the pool exists.
func (c *Config) WithDatabase(st *settings.Store) *Config {
	next := *c
	next.db = st
	return &next
}

// StoredVal is StoredValWithSource without the origin.
func StoredVal[T any](ctx context.Context, c *Config, s *Setting[T]) (T, error) {
	v, _, err := StoredValWithSource(ctx, c, s)
	return v, err
}

// StoredValWithSource answers a boot value without touching the database, so a
// locked setting never depends on a row an operator cannot see or change. The
// read gate comes first either way: whether a value is locked is no grant.
func StoredValWithSource[T any](ctx context.Context, c *Config, s *Setting[T]) (T, Origin, error) {
	var zero T
	if err := requireStored(s); err != nil {
		return zero, Origin{}, err
	}
	if err := settings.RequireRead(ctx, s.row); err != nil {
		return zero, Origin{}, err
	}
	if v, origin, ok := bootValue(c, s); ok {
		return v, origin, nil
	}
	db, err := c.database(s.Key())
	if err != nil {
		return zero, Origin{}, err
	}
	v, stored, err := settings.GetStored(ctx, db, s.row)
	if err != nil {
		return zero, Origin{}, err
	}
	return v, storedOrigin(s.Key(), stored), nil
}

// StoredValTx is StoredVal inside the caller's transaction, so several reads
// answer from one snapshot.
func StoredValTx[T any](ctx context.Context, tx pgx.Tx, c *Config, s *Setting[T]) (T, error) {
	var zero T
	if err := requireStored(s); err != nil {
		return zero, err
	}
	if err := settings.RequireRead(ctx, s.row); err != nil {
		return zero, err
	}
	if v, _, ok := bootValue(c, s); ok {
		return v, nil
	}
	return settings.GetTx(ctx, tx, s.row)
}

// StoredRequireTx is StoredValTx for a value the installation is measured in:
// with no boot value, an absent row is an error rather than the default.
func StoredRequireTx[T any](ctx context.Context, tx pgx.Tx, c *Config, s *Setting[T]) (T, error) {
	var zero T
	if err := requireStored(s); err != nil {
		return zero, err
	}
	if err := settings.RequireRead(ctx, s.row); err != nil {
		return zero, err
	}
	if v, _, ok := bootValue(c, s); ok {
		return v, nil
	}
	return settings.RequireTx(ctx, tx, s.row)
}

// SeedTx writes a bootstrap value once, unless a boot source sets the setting.
// A row written then would take effect only once that source is removed.
func SeedTx[T any](ctx context.Context, tx pgx.Tx, c *Config, s *Setting[T], v T) (stored bool, err error) {
	if err := requireStored(s); err != nil {
		return false, err
	}
	return settings.SeedValue(ctx, tx, s.row, v, c.BootLocked())
}

// BootVal is the value a boot source sets, for a raw reader that runs before a
// principal exists and so has no gate to take.
func BootVal[T any](c *Config, s *Setting[T]) (T, bool) {
	v, _, ok := bootValue(c, s)
	return v, ok
}

// ValidateJSON refuses a wire value the way a save would. A caller uses it to
// refuse a value before it writes anything else in the same transaction.
func (s *Setting[T]) ValidateJSON(raw json.RawMessage) error {
	if err := requireStored(s); err != nil {
		return err
	}
	return s.row.ValidateJSON(raw)
}

func storedOrigin(key string, stored bool) Origin {
	if stored {
		return Origin{Source: SourceDatabase, Name: key}
	}
	return Origin{Source: SourceDefault}
}

// StoredApplyTx is the machinery read inside the caller's transaction. Like
// settings.ApplyTx it takes no read gate, locked or not; the entry's
// MachineryApplied declaration is its licence.
func StoredApplyTx[T any](ctx context.Context, tx pgx.Tx, c *Config, s *Setting[T]) (T, error) {
	var zero T
	if err := requireMachinery(s); err != nil {
		return zero, err
	}
	if v, _, ok := bootValue(c, s); ok {
		return v, nil
	}
	return settings.ApplyTx(ctx, tx, s.row)
}

// StoredApplyPairTx is StoredApplyTx for two settings a decision pairs. One
// statement reads both rows, so a write that changes both together cannot
// commit between the two reads and pair values no one configured.
func StoredApplyPairTx[A, B any](ctx context.Context, tx pgx.Tx, c *Config, a *Setting[A], b *Setting[B]) (A, B, error) {
	var zeroA A
	var zeroB B
	if err := requireMachinery(a); err != nil {
		return zeroA, zeroB, err
	}
	if err := requireMachinery(b); err != nil {
		return zeroA, zeroB, err
	}
	rows, err := settings.ApplyManyTx(ctx, tx, a.row, b.row)
	if err != nil {
		return zeroA, zeroB, err
	}
	va, err := appliedValue(c, a, rows)
	if err != nil {
		return zeroA, zeroB, err
	}
	vb, err := appliedValue(c, b, rows)
	if err != nil {
		return zeroA, zeroB, err
	}
	return va, vb, nil
}

// appliedValue is s's boot value, else the row ApplyManyTx read for it.
func appliedValue[T any](c *Config, s *Setting[T], rows map[string]json.RawMessage) (T, error) {
	if v, _, ok := bootValue(c, s); ok {
		return v, nil
	}
	var v T
	if err := json.Unmarshal(rows[s.Key()], &v); err != nil {
		return v, fmt.Errorf("config: decoding %s: %w", s.Key(), err)
	}
	return v, nil
}

// requireMachinery refuses a setting the ungated readers may not read.
func requireMachinery[T any](s *Setting[T]) error {
	if err := requireStored(s); err != nil {
		return err
	}
	if !s.store.MachineryApplied {
		return fmt.Errorf("config: %s is not declared MachineryApplied; read it through StoredVal and its gate", s.Key())
	}
	return nil
}

// Save writes a stored setting through the audited settings store. The update
// gate comes before the lock, so a caller without the grant learns nothing
// about where the value comes from.
func Save[T any](ctx context.Context, c *Config, s *Setting[T], v T) error {
	if err := requireStored(s); err != nil {
		return err
	}
	if err := settings.RequireWrite(ctx, s.row); err != nil {
		return err
	}
	if locked, err := lockedAgainst(c, s, v); locked || err != nil {
		return err
	}
	db, err := c.database(s.Key())
	if err != nil {
		return err
	}
	return settings.Set(ctx, db, s.row, v)
}

// SaveTx is Save inside the caller's transaction.
func SaveTx[T any](ctx context.Context, c *Config, tx pgx.Tx, s *Setting[T], v T) error {
	if err := requireStored(s); err != nil {
		return err
	}
	if err := settings.RequireWrite(ctx, s.row); err != nil {
		return err
	}
	if locked, err := lockedAgainst(c, s, v); locked || err != nil {
		return err
	}
	db, err := c.database(s.Key())
	if err != nil {
		return err
	}
	return settings.SetTx(ctx, db, tx, s.row, v)
}

// lockedAgainst accepts the boot value itself as a no-op, so a form that sends
// every field still saves the fields that are not locked.
func lockedAgainst[T any](c *Config, s *Setting[T], v T) (bool, error) {
	return LockedChange(c, s, func(boot T) bool { return reflect.DeepEqual(boot, v) })
}

// LockedChange is the lock a save takes, for a writer with its own statement
// or its own rule of sameness. It takes no gate; the caller holds the update
// gate. locked is whether a boot source sets s. err is a LockedError unless
// same(boot) holds.
func LockedChange[T any](c *Config, s *Setting[T], same func(boot T) bool) (locked bool, err error) {
	boot, origin, ok := bootValue(c, s)
	if !ok {
		return false, nil
	}
	if same(boot) {
		return true, nil
	}
	return true, LockedError{Key: s.Key(), Origin: origin}
}

// LockFirst answers the lock of a boot-set key in place of err, a refusal of
// the value sent for it. A locked field then reads as locked whether or not
// the value sent is valid.
func (c *Config) LockFirst(key string, err error) error {
	if err == nil {
		return nil
	}
	if origin, locked := c.Locked(key); locked {
		return LockedError{Key: key, Origin: origin}
	}
	return err
}

// requireStored refuses a boot-only setting, which has no row and no gate.
func requireStored[T any](s *Setting[T]) error {
	if !s.Stored() {
		return fmt.Errorf("config: %s is not stored; read it with Val", s.Key())
	}
	return nil
}

func bootValue[T any](c *Config, s *Setting[T]) (T, Origin, bool) {
	var zero T
	if c == nil {
		return zero, Origin{}, false
	}
	r, ok := c.values[s.Key()]
	if !ok || !r.origin.FromBoot() {
		return zero, Origin{}, false
	}
	v, ok := r.value.(T)
	return v, r.origin, ok
}

func (c *Config) database(key string) (*settings.Store, error) {
	if c.db == nil {
		return nil, fmt.Errorf("config: %s is stored and this Config has no database; call WithDatabase", key)
	}
	return c.db, nil
}

// Locked reports the boot source that sets key, when one does.
func (c *Config) Locked(key string) (Origin, bool) {
	if c == nil {
		return Origin{}, false
	}
	r, ok := c.values[key]
	return r.origin, ok && r.origin.FromBoot()
}

// BootLocked are the stored keys a boot source decides: the skip set a seed
// or a reset of the settings takes.
func (c *Config) BootLocked() map[string]bool {
	out := map[string]bool{}
	if c == nil {
		return out
	}
	for _, s := range c.reg.All() {
		if _, locked := c.Locked(s.Key()); locked && s.Stored() {
			out[s.Key()] = true
		}
	}
	return out
}

// storedSetting is the typed half of a stored setting that the erased registry
// cannot name.
type storedSetting interface {
	entry() settings.Definition
	storedText(ctx context.Context, db *settings.Store) (text string, stored bool, err error)
}

// StoredDefinitions are the rows platform/settings must register.
func (c *Config) StoredDefinitions() []settings.Definition {
	var out []settings.Definition
	for _, s := range c.reg.All() {
		if stored, ok := s.(storedSetting); ok && s.Stored() {
			out = append(out, stored.entry())
		}
	}
	return out
}

// StoredEntries is Entries with each unlocked stored setting read from the
// database. Every stored entry, locked or not, takes the caller's read gate. A
// secret's value stays nil.
func (c *Config) StoredEntries(ctx context.Context) ([]Entry, error) {
	out := c.Entries()
	for i, e := range out {
		s, found := c.reg.Lookup(e.Key)
		stored, ok := s.(storedSetting)
		if !found || !ok || !e.Stored {
			continue
		}
		if err := settings.RequireRead(ctx, stored.entry()); err != nil {
			return nil, err
		}
		if e.Origin.FromBoot() {
			continue
		}
		db, err := c.database(e.Key)
		if err != nil {
			return nil, err
		}
		text, has, err := stored.storedText(ctx, db)
		if err != nil {
			return nil, err
		}
		out[i].Origin = storedOrigin(e.Key, has)
		if !e.Secret {
			out[i].Value = &text
		}
	}
	return out, nil
}
