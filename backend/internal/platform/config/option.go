// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

// Option tells "no default" apart from a zero default, which for a bool or an
// int reads the same.
type Option[T any] struct {
	v  T
	ok bool
}

// Some is an Option holding v.
func Some[T any](v T) Option[T] { return Option[T]{v: v, ok: true} }

// Get returns the value and whether there is one.
func (o Option[T]) Get() (T, bool) { return o.v, o.ok }
