// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

// Entry is one row of the configuration overview. Value is nil for a secret,
// so no consumer of the overview can leak one, and for a stored setting that
// no boot source set. A secret has no Default: the registry refuses one.
type Entry struct {
	Key, Env, Flag, Type, Doc string
	Roles                     []Role
	Secret, Stored            bool
	Value, Default            *string
	Origin                    Origin
}

// Entries lists every registered setting, in registry order, from the boot view.
func (c *Config) Entries() []Entry {
	all := c.reg.All()
	out := make([]Entry, 0, len(all))
	for _, s := range all {
		out = append(out, entryOf(s, c.values[s.Key()]))
	}
	return out
}

// Unread is the entry of a setting this program does not load: its value and
// origin are another program's, which this one cannot see.
func Unread(s Any) Entry { return entryOf(s, resolved{}) }

func entryOf(s Any, r resolved) Entry {
	e := Entry{
		Key: s.Key(), Env: s.Env(), Flag: s.Flag(), Type: s.TypeName(), Doc: s.Doc(),
		Roles: s.Roles(), Secret: s.Secret(), Stored: s.Stored(), Origin: r.origin,
	}
	if v, ok := s.defaultValue(); ok {
		if d, shown := s.format(v); shown {
			e.Default = &d
		}
	}
	if r.origin.Source == "" {
		e.Origin = Origin{Source: SourceUnset}
	}
	if s.Secret() || r.value == nil {
		return e
	}
	text, ok := s.format(r.value)
	if ok {
		e.Value = &text
	} else if e.Origin.Source == SourceDefault {
		e.Origin = Origin{Source: SourceUnset}
	}
	return e
}
