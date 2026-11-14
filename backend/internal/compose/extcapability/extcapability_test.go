// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// lane is the model boundary: it records that it was reached.
type lane struct{ calls int }

func (l *lane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: "ok"}, nil
}

var oneMessage = extension.CompleteRequest{
	Messages: []extension.Message{{Role: "user", Text: "hello"}}, NoContactData: "a fixed greeting",
}

var errRefused = errors.New("not declared")

func allow(ctx context.Context) (context.Context, error) { return ctx, nil }

func TestAModelCallTheGateRefusesNeverReachesTheLane(t *testing.T) {
	t.Parallel()
	l := &lane{}
	models := extcapability.Models{
		Lane: l, Unit: "u",
		Gate:  func(context.Context) (context.Context, error) { return nil, errRefused },
		Trace: func(context.Context) error { t.Error("a refused call was traced"); return nil },
	}
	if _, err := models.Complete(context.Background(), oneMessage); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v, want the gate's refusal", err)
	}
	if l.calls != 0 {
		t.Errorf("the lane was reached %d times", l.calls)
	}
}

func TestAModelCallThatPassesTheGateIsTracedThenAnswered(t *testing.T) {
	t.Parallel()
	l, traced := &lane{}, 0
	models := extcapability.Models{Lane: l, Unit: "u", Gate: allow, Trace: func(context.Context) error { traced++; return nil }}
	text, err := models.Complete(context.Background(), oneMessage)
	if err != nil || text != "ok" || traced != 1 || l.calls != 1 {
		t.Errorf("Complete = %q, %v; traced %d, lane %d; want ok, nil, 1, 1", text, err, traced, l.calls)
	}
	models.Lane = nil
	if _, err := models.Complete(context.Background(), oneMessage); !errors.Is(err, extcapability.ErrModelsUnwired) {
		t.Errorf("no lane bound: err = %v, want ErrModelsUnwired", err)
	}
}

// A declaration removed between the first check and the call is honoured: the
// gate is asked again just before the lane, so access does not outlive it.
func TestADeclarationRemovedDuringTheCallStopsTheCallBeforeTheLane(t *testing.T) {
	t.Parallel()
	l, asked := &lane{}, 0
	models := extcapability.Models{
		Lane: l, Unit: "u",
		Gate: func(ctx context.Context) (context.Context, error) {
			asked++
			if asked > 1 {
				return nil, errRefused
			}
			return ctx, nil
		},
		Trace: func(context.Context) error { return nil },
	}
	if _, err := models.Complete(context.Background(), oneMessage); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v, want the second check's refusal", err)
	}
	if l.calls != 0 {
		t.Errorf("the lane was reached %d times after the declaration was removed", l.calls)
	}
}

func TestARequestRefusesWhatItCannotAttribute(t *testing.T) {
	t.Parallel()
	contact := ids.NewV7().String()
	about := func(typ, id string) []extension.EntityRef { return []extension.EntityRef{{Type: typ, ID: id}} }
	cases := map[string]struct {
		req     extension.CompleteRequest
		wantErr bool
	}{
		"no messages":               {req: extension.CompleteRequest{NoContactData: "x"}, wantErr: true},
		"unknown role":              {req: extension.CompleteRequest{Messages: []extension.Message{{Role: "system", Text: "x"}}, NoContactData: "x"}, wantErr: true},
		"neither records nor why":   {req: extension.CompleteRequest{Messages: oneMessage.Messages}, wantErr: true},
		"unknown record type":       {req: extension.CompleteRequest{Messages: oneMessage.Messages, About: about("vault", contact)}, wantErr: true},
		"malformed record id":       {req: extension.CompleteRequest{Messages: oneMessage.Messages, About: about("contact", "nope")}, wantErr: true},
		"negative cap":              {req: extension.CompleteRequest{Messages: oneMessage.Messages, NoContactData: "x", MaxTokens: -1}, wantErr: true},
		"records named":             {req: extension.CompleteRequest{Messages: oneMessage.Messages, About: about("contact", contact)}},
		"no contact data, a reason": {req: oneMessage},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := extcapability.Request(tc.req)
			if tc.wantErr && !errors.Is(err, extension.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		})
	}
}

func TestPreflightRefusesWhatTheManifestCouldNotPublish(t *testing.T) {
	t.Parallel()
	good := extension.Need{Capability: extension.CapabilityModels, Reason: "Summarises a thread."}
	for name, needs := range map[string][]extension.Need{
		"unknown capability": {{Capability: "vault", Reason: "Reads tokens."}},
		"empty reason":       {{Capability: extension.CapabilityModels}},
		"declared twice":     {good, good},
	} {
		if err := extcapability.Preflight(extension.Extension{Name: "u", Needs: needs}); err == nil {
			t.Errorf("%s: preflight accepted it", name)
		}
	}
	if err := extcapability.Preflight(extension.Extension{Name: "u", Needs: []extension.Need{good}}); err != nil {
		t.Errorf("a valid declaration: %v", err)
	}
}

// inWorkspace is a context carrying a fresh workspace, and that workspace's id.
func inWorkspace() (context.Context, ids.WorkspaceID) {
	raw := ids.NewV7()
	return principal.WithWorkspaceID(context.Background(), raw), ids.From[ids.WorkspaceKind](raw)
}

func objectsFor(unit string, store blobstore.Store) extcapability.Objects {
	return extcapability.Objects{Store: store, Unit: unit, Gate: allow}
}

func TestAnObjectLivesUnderTheUnitsOwnPrefixAndNowhereElse(t *testing.T) {
	t.Parallel()
	store := blobstore.NewMemory()
	ctx, ws := inWorkspace()
	objects := objectsFor("summaries", store)
	if err := objects.Put(ctx, "report.txt", []byte("hello world")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := blobstore.WorkspaceKey(ws, "ext/summaries", "report.txt")
	body, meta, err := store.Get(ctx, want)
	if err != nil {
		t.Fatalf("the object is not at %q: %v", want, err)
	}
	if err := body.Close(); err != nil {
		t.Error(err)
	}
	if meta.ContentType != "text/plain" {
		t.Errorf("stored type = %q, want the one the bytes say, text/plain", meta.ContentType)
	}
	for _, name := range []string{"../attachment/x", "a/b", "", ".hidden", "UPPER", strings.Repeat("a", 129)} {
		if err := objects.Put(ctx, name, []byte("x")); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("Put(%q): err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestAnObjectOverTheInboundFileBoundIsRefused(t *testing.T) {
	t.Parallel()
	ctx, _ := inWorkspace()
	err := objectsFor("u", blobstore.NewMemory()).Put(ctx, "big", bytes.Repeat([]byte("a"), extension.MaxInboundFileBytes+1))
	if !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Put over %d bytes: err = %v, want ErrInvalid", extension.MaxInboundFileBytes, err)
	}
}

func TestOneUnitCannotReadAnothersObject(t *testing.T) {
	t.Parallel()
	store := blobstore.NewMemory()
	ctx, _ := inWorkspace()
	if err := objectsFor("first", store).Put(ctx, "secret", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := objectsFor("second", store).Get(ctx, "secret"); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("a second unit reading the first's object: err = %v, want ErrNotFound", err)
	}
}

func TestAnObjectCallTheGateRefusesTouchesNothing(t *testing.T) {
	t.Parallel()
	objects := extcapability.Objects{
		Store: blobstore.NewMemory(), Unit: "u",
		Gate: func(context.Context) (context.Context, error) { return nil, errRefused },
	}
	ctx := context.Background()
	_, getErr := objects.Get(ctx, "a")
	for name, err := range map[string]error{
		"Put": objects.Put(ctx, "a", []byte("x")), "Delete": objects.Delete(ctx, "a"), "Get": getErr,
	} {
		if !errors.Is(err, errRefused) {
			t.Errorf("%s: err = %v, want the gate's refusal", name, err)
		}
	}
}

// A removed unit takes its objects with it, and only its own: another unit's
// objects and the core's attachments stay. The workspace-wide sweep a data reset
// makes reaches a unit's objects too, because they sit under the workspace prefix.
func TestRemovingAUnitDeletesItsObjectsAndNoOthers(t *testing.T) {
	t.Parallel()
	store := blobstore.NewMemory()
	ctx, ws := inWorkspace()
	keep := []string{
		extcapability.Prefix(ws, "stays") + "a",
		extcapability.Prefix(ws, "gone-not") + "a",
		blobstore.WorkspaceKey(ws, "attachment", "a"),
	}
	gone := extcapability.Prefix(ws, "gone") + "a"
	for _, key := range append(keep, gone) {
		if err := store.Put(ctx, key, strings.NewReader("x"), 1, "text/plain"); err != nil {
			t.Fatal(err)
		}
	}
	composed := []extension.Extension{{Name: "stays"}, {Name: "gone-not"}}
	removed := extcapability.Gone([]string{"gone", "stays", "gone-not"}, composed)
	if len(removed) != 1 || removed[0] != "gone" {
		t.Fatalf("Gone = %v, want [gone]", removed)
	}
	if err := extcapability.Sweep(ctx, store, removed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(ctx, gone); !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("the removed unit's object survived: err = %v", err)
	}
	for _, key := range keep {
		body, _, err := store.Get(ctx, key)
		if err != nil {
			t.Errorf("%q was deleted with the removed unit: %v", key, err)
			continue
		}
		if err := body.Close(); err != nil {
			t.Error(err)
		}
	}
	if n, err := store.DeletePrefix(ctx, ws.String()+"/"); err != nil || n != len(keep) {
		t.Errorf("the workspace sweep removed %d objects, %v; want the %d left", n, err, len(keep))
	}
}
