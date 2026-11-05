// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The notice door against a real database. Who a notice reaches is decided at
// send time from live rights. The stored notice points at the record and copies
// nothing from it.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// noticeWorld adds three colleagues to the read fixture. One holds the role
// and can open every deal. One holds it and has left. One holds it but cannot
// read deals at all.
type noticeWorld struct {
	readWorld
	reader, departed, blind ids.UUID
}

func setupNotices(t *testing.T) noticeWorld {
	t.Helper()
	w := noticeWorld{readWorld: setupReads(t), reader: ids.NewV7(), departed: ids.NewV7(), blind: ids.NewV7()}
	ctx := context.Background()
	seed := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding the notice fixture: %v\n%s", err, sql)
		}
	}
	for _, u := range []ids.UUID{w.reader, w.departed, w.blind} {
		seed(`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Member')`, u, "m-"+u.String()+"@ext.test")
	}
	seed(`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, w.departed)
	seed(`INSERT INTO role (key, name, permissions) VALUES
		('deal-watcher', 'Watcher', '{"objects":{"deal":{"read":true}},"row_scope":"all"}'),
		('blind', 'Blind', '{"objects":{"company":{"read":true}},"row_scope":"all"}')`)
	for _, u := range []ids.UUID{w.reader, w.departed} {
		seed(`INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'deal-watcher'`, u)
	}
	seed(`INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = 'blind'`, w.blind)
	return w
}

func (w noticeWorld) send(t *testing.T, in extension.NoticeRequest) (extension.NoticeSent, error) {
	t.Helper()
	var sent extension.NoticeSent
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, "deal", "company"), func(ctx context.Context, core extension.Core) error {
		var err error
		sent, err = core.Notices().Send(ctx, in)
		return err
	})
	return sent, err
}

func (w noticeWorld) noticesFor(t *testing.T, user ids.UUID) (subjects []string, body string) {
	t.Helper()
	rows, err := w.owner.Query(context.Background(),
		`SELECT subject, body FROM notice WHERE recipient_user_id = $1 AND target_type = 'deal' AND target_id = $2`,
		user, w.mineDeal)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s, b string
		if err := rows.Scan(&s, &b); err != nil {
			t.Fatal(err)
		}
		subjects, body = append(subjects, s), b
	}
	return subjects, body
}

func TestARoleNoticeReachesOnlyLiveMembersWhoCanOpenTheRecord(t *testing.T) {
	w := setupNotices(t)
	sent, err := w.send(t, extension.NoticeRequest{
		Role: "deal-watcher", Record: extension.RecordRef{Type: "deal", ID: w.mineDeal.String()}, Subject: "Look at this deal",
	})
	if err != nil || sent.Recipients != 1 {
		t.Fatalf("Send = %+v, %v; want one recipient", sent, err)
	}
	if got, body := w.noticesFor(t, w.reader); len(got) != 1 || body != "" {
		t.Errorf("the reader holds %v with body %q, want one notice with no copied body", got, body)
	}
	for name, user := range map[string]ids.UUID{"departed": w.departed, "cannot read deals": w.blind} {
		sent, err := w.send(t, extension.NoticeRequest{
			Member: user.String(), Record: extension.RecordRef{Type: "deal", ID: w.mineDeal.String()}, Subject: "Look",
		})
		if err != nil || sent.Recipients != 0 {
			t.Errorf("a notice to the %s member = %+v, %v; want no recipient", name, sent, err)
		}
		if got, _ := w.noticesFor(t, user); len(got) != 0 {
			t.Errorf("the %s member was sent %v, want nothing", name, got)
		}
	}
}

func TestANoticeKeyMakesARetryASecondNoOp(t *testing.T) {
	w := setupNotices(t)
	in := extension.NoticeRequest{
		Member: w.reader.String(), Key: "stage-moved-1",
		Record: extension.RecordRef{Type: "deal", ID: w.mineDeal.String()}, Subject: "Moved",
	}
	for attempt, want := range []int{1, 0} {
		sent, err := w.send(t, in)
		if err != nil || sent.Recipients != want {
			t.Fatalf("send %d = %+v, %v; want %d newly told", attempt+1, sent, err, want)
		}
	}
	if got, _ := w.noticesFor(t, w.reader); len(got) != 1 {
		t.Errorf("a retried send left %d notices, want 1", len(got))
	}
}

func TestANoticeAboutARecordTheCallerCannotSeeIsNotFound(t *testing.T) {
	w := setupNotices(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeOwn, "company"), func(ctx context.Context, core extension.Core) error {
		_, err := core.Notices().Send(ctx, extension.NoticeRequest{
			Member: w.reader.String(), Record: extension.RecordRef{Type: "company", ID: w.theirsCompany.String()}, Subject: "x",
		})
		if !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("Send = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
