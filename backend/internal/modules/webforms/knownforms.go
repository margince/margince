// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// FormLookup reads the form a key names. known is false only for a key that
// is surely absent; kept is false for an answer that must not be cached.
type FormLookup func(ctx context.Context, key string) (f Form, known, kept bool)

// FormLister reads every live form, for the cache of real form keys.
type FormLister func(ctx context.Context) ([]Form, error)

// knownForms keeps which keys name a form. Every live form is loaded at once
// and again each refresh, so a real form never waits on a budget for lookups
// that find nothing. Each kept form expires, so a failed load cannot serve a
// form that was archived or narrowed for longer than its expiry.
type knownForms struct {
	lookup           FormLookup
	list             FormLister
	now              func() time.Time
	refresh, expires time.Duration

	mu       sync.Mutex
	forms    map[string]knownForm
	loadedAt time.Time
	absent   map[string]time.Time
}

type knownForm struct {
	form  Form
	until time.Time
}

const (
	// absentSpan is how long a key that named no form is answered from memory.
	absentSpan = 30 * time.Second
	// absentCap bounds what a flood of made-up keys can make this hold.
	absentCap = 10_000
)

func newKnownForms(lookup FormLookup, list FormLister, now func() time.Time, refresh, expires time.Duration) *knownForms {
	return &knownForms{
		lookup: lookup, list: list, now: now, refresh: refresh, expires: expires,
		forms: map[string]knownForm{}, absent: map[string]time.Time{},
	}
}

// kept answers key from memory, loading every form first when the last load
// is older than refreshSpan. ok is false for a key memory cannot answer.
func (k *knownForms) kept(ctx context.Context, key string) (f Form, exists, ok bool) {
	k.load(ctx)
	k.mu.Lock()
	defer k.mu.Unlock()
	if entry, exists := k.forms[key]; exists && k.now().Before(entry.until) {
		return entry.form, true, true
	}
	if until, absent := k.absent[key]; absent && k.now().Before(until) {
		return Form{}, false, true
	}
	return Form{}, false, false
}

func (k *knownForms) load(ctx context.Context) {
	k.mu.Lock()
	due := k.list != nil && !k.now().Before(k.loadedAt.Add(k.refresh))
	k.mu.Unlock()
	if !due {
		return
	}
	forms, err := k.list(ctx)
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	if err != nil {
		// Unexpired keys stay; an expired one is looked up alone again.
		for key, entry := range k.forms {
			if !now.Before(entry.until) {
				delete(k.forms, key)
			}
		}
		return
	}
	loaded := make(map[string]knownForm, len(forms))
	for _, f := range forms {
		loaded[f.Key] = knownForm{form: publicView(f), until: now.Add(k.expires)}
	}
	k.forms, k.loadedAt = loaded, now
}

// changed forgets key and makes the next answer load every form again, after
// a form was created, changed or archived on this replica.
func (k *knownForms) changed(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.forms, key)
	delete(k.absent, key)
	k.loadedAt = time.Time{}
}

// read looks key up and keeps the answer.
func (k *knownForms) read(ctx context.Context, key string) (Form, bool) {
	f, exists, cacheable := k.lookup(ctx, key)
	if !cacheable {
		return f, exists
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	if exists {
		k.forms[key] = knownForm{form: f, until: now.Add(k.expires)}
		return f, true
	}
	if len(k.absent) >= absentCap {
		for key, until := range k.absent {
			if !now.Before(until) {
				delete(k.absent, key)
			}
		}
	}
	if len(k.absent) < absentCap {
		k.absent[key] = now.Add(absentSpan)
	}
	return Form{}, false
}

// publicView is what the intake edge may act on: a paused form exists, with
// no origins to allow.
func publicView(f Form) Form {
	if f.State == StatePaused {
		return Form{Key: f.Key}
	}
	return f
}

// LookupForm reads the form key names for the throttle. A database fault is
// not kept and still meters.
func (h Handlers) LookupForm(ctx context.Context, key string) (Form, bool, bool) {
	f, err := h.store.PublicForm(ctx, key)
	switch {
	case err == nil:
		return f, true, true
	case errors.Is(err, apperrors.ErrNotFound):
		return Form{}, false, true
	case errors.Is(err, ErrPaused):
		return Form{Key: key}, true, true
	}
	return Form{}, true, false
}

// ListLiveForms reads every live form for the throttle's cache of real keys.
func (h Handlers) ListLiveForms(ctx context.Context) ([]Form, error) {
	return h.store.ListForms(ctx, storekit.LiveOnly)
}
