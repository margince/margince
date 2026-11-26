// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit may tell a member about a row of a record kind it declared. The notice
// points at that row by its extension kind name, and it reaches only the members
// who may read the kind.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const noticeKindTable = "ext_notes_visit"

// declareNoticeKind gives the core unit a visit kind with one row, and answers
// the row's id and the kind's name.
func (w noticeWorld) declareNoticeKind(t *testing.T) (row ids.UUID, kind string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + noticeKindTable,
		`CREATE TABLE ext.` + noticeKindTable + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz, code text NOT NULL UNIQUE)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + noticeKindTable + ` TO margince_app`,
	} {
		if _, err := w.owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the visit table (%s): %v", stmt, err)
		}
	}
	recordkinds.Register(coreUnit, []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: noticeKindTable,
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
	t.Cleanup(func() {
		recordkinds.Reset()
		if _, err := w.owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+noticeKindTable); err != nil {
			t.Errorf("dropping the visit table: %v", err)
		}
	})
	if err := w.owner.QueryRow(ctx, `INSERT INTO ext.`+noticeKindTable+` (code) VALUES ('v1') RETURNING id`).Scan(&row); err != nil {
		t.Fatal(err)
	}
	return row, extension.RecordKindName(coreUnit, "visit")
}

func (w noticeWorld) noticeCountAbout(t *testing.T, user, row ids.UUID) int {
	t.Helper()
	var n int
	if err := w.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND target_type = $2 AND target_id = $3`,
		user, extension.RecordKindObject(coreUnit, "visit"), row).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestANoticeAboutAnExtensionKindReachesOnlyMembersWhoMayReadTheKind(t *testing.T) {
	w := setupNotices(t)
	row, kind := w.declareNoticeKind(t)
	object := extension.RecordKindObject(coreUnit, "visit")
	if err := identity.RegisterRbacObjects(identity.RbacObject(object)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(identity.ResetRbacObjectsForTest)
	if _, err := w.owner.Exec(context.Background(), `INSERT INTO role (key, name, permissions) VALUES
		('visit-watcher', 'Visit watcher', jsonb_build_object('objects', jsonb_build_object($1::text, '{"read":true}'::jsonb), 'row_scope', 'all'))`, object); err != nil {
		t.Fatal(err)
	}
	if _, err := w.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'visit-watcher'`, w.reader); err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{w.reader, w.blind} {
		err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, object), func(ctx context.Context, core extension.Core) error {
			_, err := core.Notices().Send(ctx, extension.NoticeRequest{
				Member: user.String(), Record: extension.RecordRef{Type: kind, ID: row.String()}, Subject: "A visit",
			})
			return err
		})
		if err != nil {
			t.Fatalf("sending about the %s row to %s: %v", kind, user, err)
		}
	}
	if got := w.noticeCountAbout(t, w.reader, row); got != 1 {
		t.Errorf("the member who may read the kind has %d notices, want 1", got)
	}
	if got := w.noticeCountAbout(t, w.blind, row); got != 0 {
		t.Errorf("the member who may not read the kind has %d notices, want 0", got)
	}
}

func TestANoticeAboutAKindTheUnitDidNotDeclareIsInvalid(t *testing.T) {
	w := setupNotices(t)
	row, _ := w.declareNoticeKind(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, "deal"), func(ctx context.Context, core extension.Core) error {
		_, err := core.Notices().Send(ctx, extension.NoticeRequest{
			Member: w.reader.String(), Record: extension.RecordRef{Type: extension.RecordKindName("another", "visit"), ID: row.String()}, Subject: "x",
		})
		return err
	})
	if !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a notice about another unit's kind = %v, want ErrInvalid", err)
	}
}
