// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package cliflags registers command-line flags whose value may come from the
// environment, and applies the environment only where the flag was not given.
//
// It exists because `flag` echoes a non-empty default in its usage output as
// `(default "…")`. Wiring os.Getenv straight into a flag's default therefore puts
// whatever the environment supplied into the usage text, and every binary here
// prints that text on a mistyped flag or `-h`. The values in question are DSNs,
// signing keys, OAuth client secrets and bearer tokens; on CI that stderr is a
// public build log, and in a container it is the pod's log stream.
//
// Registering the LITERAL default and resolving the environment after Parse keeps
// the value out of the usage text while leaving the precedence unchanged: an
// explicit flag still wins over the environment, which still wins over the
// literal.
//
// The obligation is derived, not maintained: each binary asserts that its own
// usage output contains no value from any MARGINCE_* variable it reads, so a flag
// added later cannot quietly reintroduce the leak by being left off a list.
package cliflags

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
)

// Namespace prefixes every variable this tree reads, and is the prefix a
// conventional binding's variable carries (see Shortfalls).
const Namespace = "MARGINCE_"

// Env collects the flag-to-environment bindings of one FlagSet.
type Env struct {
	bindings []binding
}

type binding struct {
	name string
	env  string
	kind config.Kind
	// set parses an environment value into the flag's target, refusing one
	// its kind cannot read.
	set func(string) error
}

// String registers name on fs with its literal default — empty when only the
// environment supplies a value — and records env as that flag's source. The
// literal is safe to echo; the environment's value is not, which is the whole
// reason the two are separated here.
func (e *Env) String(fs *flag.FlagSet, target *string, name, env, literal, usage string) {
	fs.StringVar(target, name, literal, usage)
	e.bind(fs, name, env, config.KindString)
}

// Duration is String for a time.Duration flag ("15m", "24h").
func (e *Env) Duration(fs *flag.FlagSet, target *time.Duration, name, env string, literal time.Duration, usage string) {
	fs.DurationVar(target, name, literal, usage)
	e.bind(fs, name, env, config.KindDuration)
}

// Int is String for an integer flag.
func (e *Env) Int(fs *flag.FlagSet, target *int, name, env string, literal int, usage string) {
	fs.IntVar(target, name, literal, usage)
	e.bind(fs, name, env, config.KindInt)
}

// Bool is String for a boolean flag. A value the flag cannot read is refused
// rather than taken as false: an operator who typed "yes" meant something, and
// booting with the opposite would hide it.
func (e *Env) Bool(fs *flag.FlagSet, target *bool, name, env string, literal bool, usage string) {
	fs.BoolVar(target, name, literal, usage)
	e.bind(fs, name, env, config.KindBool)
}

// bind records env as the source of a flag just registered on fs. The value is
// parsed by the flag's own Value, so the flag and its variable read one text
// one way — "010" or "0x10" means the same number from either.
func (e *Env) bind(fs *flag.FlagSet, name, env string, kind config.Kind) {
	value := fs.Lookup(name).Value
	e.bindings = append(e.bindings, binding{name: name, env: env, kind: kind, set: func(v string) error {
		if err := value.Set(v); err != nil {
			return fmt.Errorf("%s=%q is not a valid %s", env, v, kind)
		}
		return nil
	}})
}

// Apply fills every registered flag the caller did not pass from its environment
// variable. Call it immediately after fs.Parse.
//
// An empty environment value is treated as unset, matching .env.example's
// promise that "an empty value is treated as unset, so a blank line is safe" —
// otherwise a blank line in a sourced env file would erase a literal default.
//
// A value its flag's kind cannot read is an error naming the variable, never a
// silent fallback to the literal: an operator who typed a value and got the
// default instead would have nothing to tell them it never took. Every such
// fault is returned together, so a boot reports them all at once.
func (e *Env) Apply(fs *flag.FlagSet, getenv func(string) string) error {
	given := make(map[string]bool, fs.NFlag())
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	var faults []error
	for _, b := range e.bindings {
		if given[b.name] {
			continue
		}
		if v := getenv(b.env); v != "" {
			if err := b.set(v); err != nil {
				faults = append(faults, err)
			}
		}
	}
	return errors.Join(faults...)
}

// EnvKeys returns the environment variables this Env reads, so a test can seed
// every one of them without maintaining a second copy of the list.
func (e *Env) EnvKeys() []string {
	keys := make([]string, 0, len(e.bindings))
	for _, b := range e.bindings {
		keys = append(keys, b.env)
	}
	return keys
}

// Shortfalls says what keeps a role's flags from being set through its
// container's environment: a flag no variable reaches and keptOff does not
// excuse, and a variable not named Namespace and the flag upper-cased — one
// spelling rule is what lets an operator find it without looking it up.
//
// keptOff answers for the flags a role keeps command-line-only on purpose; the
// role's test supplies it from a waiver set, which holds each one's reason.
func (e *Env) Shortfalls(fs *flag.FlagSet, keptOff func(flag string) bool) []string {
	bound := make(map[string]bool, len(e.bindings))
	var out []string
	for _, b := range e.bindings {
		bound[b.name] = true
		if b.env != Namespace+strings.ToUpper(strings.ReplaceAll(b.name, "-", "_")) {
			out = append(out, fmt.Sprintf("--%s is read from %s, not %s<FLAG>", b.name, b.env, Namespace))
		}
	}
	fs.VisitAll(func(f *flag.Flag) {
		if !bound[f.Name] && !keptOff(f.Name) {
			out = append(out, fmt.Sprintf("--%s has no environment variable: bind it, or keep it flag-only with a reason", f.Name))
		}
	})
	return out
}

// Items describes the flags registered on fs as configuration items, so a role
// that already declares its surface once — as flags with defaults and usage —
// does not declare it a second time as data.
//
// Name, default and doc are READ BACK from the FlagSet rather than restated:
// the usage text an operator sees on `-h` and the doc a generated template
// carries are then the same sentence by construction, and cannot drift.
//
// public names the bindings whose values are safe to echo. EVERYTHING ELSE IS
// TREATED AS A SECRET, and that direction is the point: a map miss must not
// mean "publish it". A flag added later and classified by nobody is withheld
// until somebody decides it is safe — the failure an operator can recover from.
// The other direction puts a bearer token in a build log and cannot be undone.
//
// The caller supplies it because only the role knows which of its own STRING
// flags carry a DSN, a signing key or a bearer token; the mechanism here cannot
// tell a path from a password. A duration, a number or a switch can be told
// apart by its kind alone — none of them authenticates anybody — so only a
// string binding needs the role's word that it is safe.
func (e *Env) Items(fs *flag.FlagSet, role string, public map[string]bool) []config.Item {
	registered := make(map[string]*flag.Flag, len(e.bindings))
	fs.VisitAll(func(f *flag.Flag) { registered[f.Name] = f })

	items := make([]config.Item, 0, len(e.bindings))
	for _, b := range e.bindings {
		f := registered[b.name]
		items = append(items, config.Item{
			Name:     b.env,
			FlagName: b.name,
			Kind:     b.kind,
			Default:  f.DefValue,
			Secret:   b.kind == config.KindString && !public[b.env],
			Roles:    []string{role},
			Doc:      f.Usage,
		})
	}
	return items
}
