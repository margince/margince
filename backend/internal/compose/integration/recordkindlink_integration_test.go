// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const probeKind = "ext:kindlink:thing"

// kindFixture declares one extension kind over a real ext table. It seeds a live
// and an archived row, so the probes read what a unit's own table holds.
func kindFixture(t *testing.T) (live, archived ids.UUID) {
	t.Helper()
	kindTable(t, "ext_kindlink_thing", `id uuid PRIMARY KEY, archived_at timestamptz, code text NOT NULL UNIQUE`)
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("kindlink", []extension.RecordKindDeclaration{{
		Kind: "thing", DisplayName: "Thing", Table: "ext_kindlink_thing",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
	live, archived = ids.NewV7(), ids.NewV7()
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO ext.ext_kindlink_thing (id, code, archived_at) VALUES ($1, 'A', NULL), ($2, 'B', now())`,
		live, archived); err != nil {
		t.Fatalf("seeding the kind's rows: %v", err)
	}
	return live, archived
}

func probe(ctx context.Context, e *Env, id ids.UUID, f func(context.Context, pgx.Tx, string, ids.UUID) error) error {
	return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { return f(ctx, tx, probeKind, id) })
}

func TestALinkToAnExtensionRecordNeedsALiveRowAndTheKindsReadRight(t *testing.T) {
	e := Setup(t)
	live, archived := kindFixture(t)
	reader := e.As(e.Rep1, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"ext_kindlink_thing": {Read: true}},
	})
	denied := e.As(e.Rep1, nil, principal.Permissions{RowScope: principal.RowScopeAll})

	for name, probeFn := range map[string]func(context.Context, pgx.Tx, string, ids.UUID) error{
		"link":   auth.EnsureLinkTarget,
		"attach": auth.EnsureAttachTarget,
	} {
		if err := probe(reader, e, live, probeFn); err != nil {
			t.Errorf("%s: a live row of a kind the caller may read was refused: %v", name, err)
		}
		if err := probe(reader, e, archived, probeFn); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: an archived row = %v, want ErrNotFound", name, err)
		}
		if err := probe(reader, e, ids.NewV7(), probeFn); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: a row nobody holds = %v, want ErrNotFound", name, err)
		}
		if err := probe(denied, e, live, probeFn); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: a caller with no read right on the kind = %v, want ErrNotFound so existence stays hidden", name, err)
		}
	}
}

func kindWorker(e *Env, kindGrant principal.ObjectGrant) context.Context {
	full := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return e.As(e.Rep1, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"tag": full, "list": full, "ext_kindlink_thing": kindGrant},
	})
}

func TestAnExtensionRecordTakesATagAndJoinsAShortlistUnderTheKindsRights(t *testing.T) {
	e := Setup(t)
	live, archived := kindFixture(t)
	store := collections.NewStore(e.DB())
	worker := kindWorker(e, principal.ObjectGrant{Read: true, Update: true})

	tag, err := store.CreateTag(worker, "Kind tag", nil, nil)
	if err != nil {
		t.Fatalf("creating the tag: %v", err)
	}
	applied, err := store.ApplyTag(worker, tag.ID, probeKind, live)
	if err != nil {
		t.Fatalf("tagging an extension record: %v", err)
	}
	if applied.EntityType != probeKind {
		t.Errorf("the tagging names %q, want %q", applied.EntityType, probeKind)
	}
	if _, err := store.ApplyTag(worker, tag.ID, probeKind, archived); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("tagging an archived extension record = %v, want ErrNotFound", err)
	}
	if _, err := store.ApplyTag(worker, tag.ID, "ext:kindlink:ghost", live); err == nil {
		t.Error("tagging a kind no unit declared was accepted")
	}

	list, err := store.CreateList(worker, collections.CreateListInput{Name: "Kind list", EntityType: probeKind})
	if err != nil {
		t.Fatalf("creating a Shortlist of an extension kind: %v", err)
	}
	if _, err := store.AddMember(worker, list.ID, collections.MemberChange{
		EntityType: probeKind, EntityID: live, Reason: collections.ReasonChosen,
	}); err != nil {
		t.Fatalf("adding an extension record to the Shortlist: %v", err)
	}
	// The readers walk the same rows by entity type. A kind they did not learn
	// would surface here as an error or a silent omission.
	if _, _, err := store.GetTag(worker, tag.ID); err != nil {
		t.Errorf("reading a tag that sits on an extension record: %v", err)
	}
	if members, _, err := store.ListMembers(worker, list.ID, 50, ""); err != nil || len(members) != 1 {
		t.Errorf("the Shortlist's members = %d, %v; want the one added", len(members), err)
	}
}

func TestAnExtensionKindWithoutTheUpdateRightCannotBeTagged(t *testing.T) {
	e := Setup(t)
	live, _ := kindFixture(t)
	store := collections.NewStore(e.DB())
	readOnly := kindWorker(e, principal.ObjectGrant{Read: true})
	tag, err := store.CreateTag(readOnly, "Kind tag", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyTag(readOnly, tag.ID, probeKind, live); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a kind granted only read took a tag: %v", err)
	}
}

// The audit door takes an extension kind's name as it takes a core one. The rule
// it records names the kind, so a reader can tell what the change was on.
func TestAnAuditRowNamesAnExtensionKindAndTheRuleThatAdmittedIt(t *testing.T) {
	e := Setup(t)
	live, _ := kindFixture(t)
	worker := kindWorker(e, principal.ObjectGrant{Read: true, Update: true})
	var auditID ids.UUID
	err := database.WithWorkspaceTx(worker, e.Pool, func(tx pgx.Tx) error {
		var err error
		auditID, err = storekit.AuditEvent(worker, tx, "update", probeKind, live, map[string]any{"code": "A"})
		return err
	})
	if err != nil {
		t.Fatalf("auditing a change to an extension record: %v", err)
	}
	var entityType, rule string
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT entity_type, authorization_rule FROM audit_log WHERE id = $1`, auditID).Scan(&entityType, &rule); err != nil {
		t.Fatal(err)
	}
	if entityType != probeKind {
		t.Errorf("audit entity_type = %q, want %q", entityType, probeKind)
	}
	if !strings.Contains(rule, probeKind) {
		t.Errorf("authorization_rule = %q, want it to name %q", rule, probeKind)
	}
}

func TestALinkToAnUndeclaredExtensionKindIsRefused(t *testing.T) {
	e := Setup(t)
	kindFixture(t)
	admin := e.Admin()
	err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return auth.EnsureLinkTarget(admin, tx, "ext:kindlink:ghost", ids.NewV7())
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an undeclared kind = %v, want ErrNotFound", err)
	}
}
