// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/settings"
)

type Role string

const (
	RoleAPI     Role = "api"
	RoleWorker  Role = "worker"
	RoleMigrate Role = "migrate"
)

type Spec[T any] struct {
	Key      string
	Env      string
	Flag     string
	Type     Type[T]
	Default  Option[T]
	Min, Max Option[T]
	Validate func(T) error
	Secret   bool
	Roles    []Role
	Doc      string
}

// Store is what a stored setting needs beyond its Spec: who may change it and
// how platform/settings treats its row.
type Store struct {
	RBACObject       string
	AuditVerb        string
	Freeze           func(context.Context, pgx.Tx) (bool, string, error)
	MachineryApplied bool
	SecretReference  bool
}

type Setting[T any] struct {
	spec  Spec[T]
	store *Store
	row   *settings.Entry[T]
}

func Define[T any](s Spec[T]) *Setting[T] { return &Setting[T]{spec: s} }

// DefineStored declares a setting an administrator may also change at run
// time. platform/settings keeps storing it, through an entry built here so the
// bounds in the Spec are the ones that guard a write.
func DefineStored[T any](s Spec[T], st Store) *Setting[T] {
	setting := &Setting[T]{spec: s, store: &st}
	def, _ := s.Default.Get()
	row := settings.Define(s.Key, st.RBACObject, st.AuditVerb, def, setting.checkTyped)
	if st.Freeze != nil {
		row = row.WithFreeze(st.Freeze)
	}
	if st.MachineryApplied {
		row = row.MachineryApplied()
	}
	if st.SecretReference {
		row = row.AsSecretReference()
	}
	setting.row = row
	return setting
}

func (s *Setting[T]) Spec() Spec[T] { return s.spec }
func (s *Setting[T]) Stored() bool  { return s.store != nil }
func (s *Setting[T]) Key() string   { return s.spec.Key }
func (s *Setting[T]) Env() string   { return s.spec.Env }
func (s *Setting[T]) Flag() string  { return s.spec.Flag }
func (s *Setting[T]) Doc() string   { return s.spec.Doc }
func (s *Setting[T]) Secret() bool  { return s.spec.Secret }
func (s *Setting[T]) Roles() []Role { return s.spec.Roles }

// TypeName tolerates a missing Type so the registry can report it instead of
// panicking while it describes the fault.
func (s *Setting[T]) TypeName() string {
	if s.spec.Type == nil {
		return ""
	}
	return s.spec.Type.Name()
}

func (s *Setting[T]) entry() settings.Definition { return s.row }

func (s *Setting[T]) storedText(ctx context.Context, db *settings.Store) (string, error) {
	v, err := settings.Get(ctx, db, s.row)
	if err != nil {
		return "", err
	}
	return s.spec.Type.Format(v), nil
}

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

// checkTyped never puts the value in its message, only the limits, so a secret
// cannot leak through a refusal.
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
	if s.spec.Validate != nil {
		return s.spec.Validate(v)
	}
	return nil
}

func (s *Setting[T]) invariantFault() error {
	if s.spec.Type == nil {
		return fmt.Errorf("config: %s declares no type", s.spec.Key)
	}
	_, hasDefault := s.spec.Default.Get()
	if s.spec.Secret && hasDefault {
		return fmt.Errorf("config: %s is a secret with a default; a default is printed in docs", s.spec.Key)
	}
	_, hasMin := s.spec.Min.Get()
	_, hasMax := s.spec.Max.Get()
	var zero T
	if _, comparable := s.spec.Type.Less(zero, zero); (hasMin || hasMax) && !comparable {
		return fmt.Errorf("config: %s sets Min or Max on a %s", s.spec.Key, s.spec.Type.Name())
	}
	if def, ok := s.spec.Default.Get(); ok {
		if err := s.checkTyped(def); err != nil {
			return fmt.Errorf("config: %s: its own default is invalid: %w", s.spec.Key, err)
		}
	}
	return nil
}
