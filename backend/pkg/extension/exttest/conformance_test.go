// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package exttest_test

// The conformance fixture: one probe per capability of the published Runtime,
// run against the kit. Each probe must answer the sentinel the core answers to
// a unit that holds nothing, so the kit cannot drift from the core's refusals
// unnoticed. The roster test fails when Runtime gains a method with no probe,
// which is what keeps the fixture covering every frozen capability.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/exttest"
)

var probes = map[string]func(ctx context.Context, rt extension.Runtime) error{
	"Secrets": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Secrets().GetUser(ctx, "u", "k")
		return err
	},
	"Tx": func(ctx context.Context, rt extension.Runtime) error {
		return rt.Tx(ctx, func(context.Context, extension.Tx) error { return nil })
	},
	"Caller": func(_ context.Context, rt extension.Runtime) error {
		if rt.Caller().Type != extension.CallerSystem {
			return errors.New("the default caller is not a job tick")
		}
		return extension.ErrNotDeclared
	},
	"Ingest": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Ingest(ctx, "u", extension.Record{})
		return err
	},
	"IngestBatch": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.IngestBatch(ctx, "u", "s", nil, "")
		return err
	},
	"IngestPosition": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.IngestPosition(ctx, "s")
		return err
	},
	"IngestMemberBatch": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.IngestMemberBatch(ctx, "u", "s", nil, "", 0)
		return err
	},
	"MemberIngestPosition": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.MemberIngestPosition(ctx, "u", "s")
		return err
	},
	"SyncNow": func(ctx context.Context, rt extension.Runtime) error { return rt.SyncNow(ctx, "j") },
	"OAuthAccessToken": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.OAuthAccessToken(ctx, "p")
		return err
	},
	"OAuthDisconnect": func(ctx context.Context, rt extension.Runtime) error { return rt.OAuthDisconnect(ctx, "p") },
	"Deals": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Deals().Get(ctx, "d")
		return err
	},
	"Authority": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Authority().CanWrite(ctx, "u", extension.EntityRef{})
		return err
	},
	"Directory": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Directory().Emails(ctx, nil)
		return err
	},
	"Activities": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Activities().Attachments(ctx, "u", "m")
		return err
	},
	"SetAside": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.SetAside().Contact(ctx, "u", "c")
		return err
	},
	"Companies": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Companies().Get(ctx, "u", "c")
		return err
	},
	"MailContent": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.MailContent().Get(ctx, "u", "m")
		return err
	},
	"Models": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Models().Complete(ctx, "t", extension.CompleteRequest{})
		return err
	},
	"Objects": func(ctx context.Context, rt extension.Runtime) error {
		_, err := rt.Objects().Get(ctx, "o")
		return err
	},
}

// refused is the sentinel each probe must answer on a unit that holds nothing.
var refused = map[string]error{
	"Secrets": extension.ErrSecretNotFound, "Tx": extension.ErrRuntimeExpired, "Caller": extension.ErrNotDeclared,
	"Ingest": extension.ErrAttendedIngest, "IngestBatch": extension.ErrAttendedIngest,
	"IngestPosition":    extension.ErrNotDeclared,
	"IngestMemberBatch": extension.ErrAttendedIngest, "MemberIngestPosition": extension.ErrNotDeclared, "SyncNow": extension.ErrNoSuchJob,
	"OAuthAccessToken": extension.ErrNoSuchOAuthApp, "OAuthDisconnect": extension.ErrNoSuchOAuthApp,
	"Deals": extension.ErrAttendedRead, "Authority": extension.ErrAttendedRead, "Directory": extension.ErrAttendedRead,
	"Activities": extension.ErrAttendedRead, "SetAside": extension.ErrAttendedRead,
	"Companies": extension.ErrAttendedRead, "MailContent": extension.ErrAttendedRead,
	"Models": extension.ErrNotDeclared, "Objects": extension.ErrNotDeclared,
}

func TestEveryRuntimeCapabilityHasAProbe(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeFor[extension.Runtime]()
	for m := range rt.Methods() {
		if _, ok := probes[m.Name]; !ok {
			t.Errorf("Runtime.%s has no conformance probe: add one to probes and refused", m.Name)
		}
	}
	if len(probes) != rt.NumMethod() || len(refused) != rt.NumMethod() {
		t.Errorf("the probes (%d) and refusals (%d) must match Runtime's %d methods", len(probes), len(refused), rt.NumMethod())
	}
}

func TestKitRuntimeRefusesEveryCapability(t *testing.T) {
	t.Parallel()
	for name, probe := range probes {
		if err := probe(t.Context(), exttest.Runtime{}); !errors.Is(err, refused[name]) {
			t.Errorf("%s answered %v, want %v", name, err, refused[name])
		}
	}
}

// An override reaches the probe through an embedded kit, which is how a unit's
// fake is built.
type granted struct{ exttest.Runtime }

func (granted) SyncNow(context.Context, extension.JobName) error { return nil }

func TestEmbeddedKitLetsAFakeOverrideOneCapability(t *testing.T) {
	t.Parallel()
	if err := probes["SyncNow"](t.Context(), granted{}); err != nil {
		t.Errorf("the override was not used: %v", err)
	}
	if err := probes["Objects"](t.Context(), granted{}); !errors.Is(err, extension.ErrNotDeclared) {
		t.Errorf("an embedded kit stopped refusing what it was not given: %v", err)
	}
}

func TestKitCoreHandsOutEveryDoor(t *testing.T) {
	t.Parallel()
	var core extension.Core = exttest.Core{}
	if core.Activities() != nil || core.Notices() != nil {
		t.Error("the kit core hands back a door it should leave nil")
	}
}
