// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// Role is a program that reads a setting.
type Role string

// The programs.
const (
	RoleAPI     Role = "api"
	RoleWorker  Role = "worker"
	RoleMigrate Role = "migrate"
)

// Refusal is the line a program prints when it stops on err. Most of its errors
// already open with the program's name, so the name is added only when missing.
func (r Role) Refusal(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, string(r)+": ") {
		return msg
	}
	return string(r) + ": " + msg
}

// Spec declares a setting: its names, type, default, bounds and doc.
type Spec[T any] struct {
	Key      string
	Env      string
	Flag     string
	Type     Type[T]
	Default  Option[T]
	Min, Max Option[T]
	Validate func(T) error
	Secret   bool
	// AllowNone lets a secret be declared absent with ${none}: it then reads
	// as empty, and its Origin says None.
	AllowNone bool
	Roles     []Role
	Doc       string
}

// Store is what a stored setting needs beyond its Spec: who may change it and
// how platform/settings treats its row.
type Store struct {
	RBACObject       string
	AuditVerb        string
	Freeze           func(context.Context, pgx.Tx) (bool, string, error)
	MachineryApplied bool
	// SecretReference stores a Secret's value sealed in the key vault: the row
	// holds the vault's ref, Save seals and StoredVal opens.
	SecretReference bool
	// SurvivesDataReset keeps the row through a data reset: the installation's
	// own identity, which a reset of its records must not take with them.
	SurvivesDataReset bool
}

// Setting is a declared setting, read through Val or StoredVal.
type Setting[T any] struct {
	spec  Spec[T]
	store *Store
	row   *settings.Entry[T]
	// sealed is a SecretReference setting's row in place of row: the vault
	// ref, never the value.
	sealed *settings.Entry[string]
}

// Define declares a setting only boot sources set.
func Define[T any](s Spec[T]) *Setting[T] { return &Setting[T]{spec: s} }

// DefineStored declares a setting an administrator may also change at run
// time. platform/settings keeps storing it, through an entry built here so the
// bounds in the Spec are the ones that guard a write.
func DefineStored[T any](s Spec[T], st Store) *Setting[T] {
	setting := &Setting[T]{spec: s, store: &st}
	if st.SecretReference {
		setting.sealed = sealedEntry(s.Key, st)
		return setting
	}
	def, _ := s.Default.Get()
	row := settings.Define(s.Key, st.RBACObject, st.AuditVerb, def, setting.checkTyped)
	if st.Freeze != nil {
		row = row.WithFreeze(st.Freeze)
	}
	if st.MachineryApplied {
		row = row.MachineryApplied()
	}
	if st.SurvivesDataReset {
		row = row.AsInstallationIdentity()
	}
	setting.row = row
	return setting
}

// Spec is the declaration itself.
func (s *Setting[T]) Spec() Spec[T] { return s.spec }

// Stored is true for a setting an administrator may change at run time.
func (s *Setting[T]) Stored() bool { return s.store != nil }

// Key is the dotted name, also the YAML path.
func (s *Setting[T]) Key() string { return s.spec.Key }

// Env is the environment variable name.
func (s *Setting[T]) Env() string { return s.spec.Env }

// Flag is the command-line flag without dashes, or "".
func (s *Setting[T]) Flag() string { return s.spec.Flag }

// Doc is the operator-facing description.
func (s *Setting[T]) Doc() string { return s.spec.Doc }

// Secret is true for a value never shown or logged.
func (s *Setting[T]) Secret() bool { return s.spec.Secret }

func (s *Setting[T]) allowNone() bool { return s.spec.AllowNone }

// Roles are the programs that read the setting.
func (s *Setting[T]) Roles() []Role { return s.spec.Roles }

// TypeName tolerates a missing Type so the registry can report it instead of
// panicking while it describes the fault.
func (s *Setting[T]) TypeName() string {
	if s.spec.Type == nil {
		return ""
	}
	return s.spec.Type.Name()
}

//nolint:ireturn // the erased registry reaches every stored entry through settings.Definition
func (s *Setting[T]) entry() settings.Definition {
	if s.sealed != nil {
		return s.sealed
	}
	return s.row
}

// storedText never opens a sealed value: a secret is shown nowhere, so only
// whether a row exists is read.
func (s *Setting[T]) storedText(ctx context.Context, db *settings.Store) (string, bool, error) {
	if s.sealed != nil {
		_, stored, err := settings.GetStored(ctx, db, s.sealed)
		return "", stored, err
	}
	v, stored, err := settings.GetStored(ctx, db, s.row)
	if err != nil {
		return "", false, err
	}
	return s.spec.Type.Format(v), stored, nil
}

// DefaultText is the default as an operator writes it.
func (s *Setting[T]) DefaultText() (string, bool) {
	v, ok := s.spec.Default.Get()
	if !ok {
		return "", false
	}
	return s.spec.Type.Format(v), true
}

// defaultValue hands the loader the typed default, because formatting it to
// text and parsing it back does not round-trip every struct.
//
//craft:ignore naked-any the registry holds settings of many types behind one erased view
func (s *Setting[T]) defaultValue() (any, bool) { return s.spec.Default.Get() }

//craft:ignore naked-any the registry holds settings of many types behind one erased view
func (s *Setting[T]) parseText(text string) (any, error) { return s.spec.Type.Parse(text) }

//craft:ignore naked-any the registry holds settings of many types behind one erased view
func (s *Setting[T]) decodeYAML(n *yaml.Node) (any, error) { return s.spec.Type.DecodeYAML(n) }

//craft:ignore naked-any the registry holds settings of many types behind one erased view
func (s *Setting[T]) format(v any) string {
	typed, ok := v.(T)
	if !ok {
		return ""
	}
	return s.spec.Type.Format(typed)
}

//craft:ignore naked-any the registry holds settings of many types behind one erased view
func (s *Setting[T]) check(v any) error {
	typed, ok := v.(T)
	if !ok {
		return fmt.Errorf("holds a %T, not this setting's type", v)
	}
	return s.checkTyped(typed)
}

// checkTyped never puts the value in its message, only the limits. A secret's
// Validate is hand-written and may format its input, so its reason is dropped.
func (s *Setting[T]) checkTyped(v T) error {
	if limit, ok := s.spec.Min.Get(); ok {
		if less, _ := s.spec.Type.Less(v, limit); less {
			return fmt.Errorf("must be at least %s", s.spec.Type.Format(limit))
		}
	}
	if limit, ok := s.spec.Max.Get(); ok {
		if less, _ := s.spec.Type.Less(limit, v); less {
			return fmt.Errorf("must be at most %s", s.spec.Type.Format(limit))
		}
	}
	if s.spec.Validate == nil {
		return nil
	}
	if err := s.spec.Validate(v); err != nil {
		if s.spec.Secret {
			return errInvalidSecret
		}
		return err
	}
	return nil
}

func (s *Setting[T]) invariantFault() error {
	if s.spec.Type == nil {
		return fmt.Errorf("config: %s declares no type", s.spec.Key)
	}
	_, hasDefault := s.spec.Default.Get()
	if s.spec.AllowNone && !s.spec.Secret {
		return fmt.Errorf("config: %s allows ${none} but is not a secret; only a secret reads it", s.spec.Key)
	}
	if s.spec.Secret && hasDefault {
		return fmt.Errorf("config: %s is a secret with a default; a default is printed in docs", s.spec.Key)
	}
	if err := s.storeFault(); err != nil {
		return err
	}
	_, hasMin := s.spec.Min.Get()
	_, hasMax := s.spec.Max.Get()
	var zero T
	if _, ordered := s.spec.Type.Less(zero, zero); (hasMin || hasMax) && !ordered {
		return fmt.Errorf("config: %s sets Min or Max on a %s", s.spec.Key, s.spec.Type.Name())
	}
	if def, ok := s.spec.Default.Get(); ok {
		if err := s.checkTyped(def); err != nil {
			return fmt.Errorf("config: %s: its own default is invalid: %w", s.spec.Key, err)
		}
	}
	return nil
}

// storeFault refuses a stored secret whose row would hold it in the clear. It
// also refuses a sealed row that a gate-free machinery read would hand out.
func (s *Setting[T]) storeFault() error {
	if s.store == nil {
		return nil
	}
	switch {
	case s.spec.Secret && !s.store.SecretReference:
		return fmt.Errorf("config: %s is a stored secret without SecretReference; its row would hold the secret in the clear", s.spec.Key)
	case s.store.SecretReference && !s.spec.Secret:
		return fmt.Errorf("config: %s is a SecretReference but not a Secret; only a secret is sealed", s.spec.Key)
	case s.store.SecretReference && s.store.MachineryApplied:
		return fmt.Errorf("config: %s is sealed and MachineryApplied; a sealed value is read only behind its gate", s.spec.Key)
	}
	return nil
}
