// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package systemmail

// What changes while an open fetches the bytes, what a failed read leaves
// behind, and which day's opens the cap counts.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// changingStore runs change before it hands out the bytes, as another session
// would while the open reads them.
type changingStore struct {
	blobstore.Store
	change func()
}

func (s changingStore) Get(ctx context.Context, key string) (io.ReadCloser, blobstore.Object, error) {
	s.change()
	return s.Store.Get(ctx, key)
}

// opensOn writes a unit's opens of the day the SQL expression at names, as the
// export rows an open leaves.
func (e filesEnv) opensOn(t *testing.T, unit, at string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO system_log (actor_type, actor_id, action, detail, occurred_at)
		SELECT 'system', 'system', 'export', jsonb_build_object('kind', $1::text, 'extension', $2::text), `+at+`
		  FROM generate_series(1, $3)`, Opened, unit, extension.MaxAttachmentOpensPerDay)
}

func TestAFileHiddenWhileItsBytesAreReadIsNotGiven(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	ctx := e.reader(t, principal.RowScopeAll)
	for name, hide := range map[string]string{
		"withheld":      `UPDATE attachment SET bytes_withheld = true WHERE id = $1`,
		"archived":      `UPDATE attachment SET archived_at = now() WHERE id = $1`,
		"mail archived": `UPDATE activity SET archived_at = now() WHERE id = (SELECT activity_id FROM attachment WHERE id = $1)`,
		"mail restricted": `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
			SELECT p.contact_id, 'processing_restriction', 'test', 'human:test', 'subject'
			  FROM activity_participant p JOIN attachment at ON at.activity_id = p.activity_id WHERE at.id = $1`,
	} {
		mail, contact := e.mail(t, "workspace"), ids.NewV7()
		e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by, visibility) VALUES ($1, 'Sender', 'manual', 'system', 'workspace')`, contact)
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`, mail, contact)
		file := e.file(t, mail, name)
		files := e.files("mirror")
		files.Blob = changingStore{Store: e.blob, change: func() { e.WsExec(t, hide, file) }}
		if got, err := e.open(ctx, t, files, file); !errors.Is(err, extension.ErrNotFound) || got.Content != nil {
			t.Errorf("a file %s during the read: %d bytes, err = %v; want no bytes and ErrNotFound", name, len(got.Content), err)
		}
	}
	if rows := e.exports(t, Opened); rows != 0 {
		t.Errorf("opens refused after the read left %d export rows, want 0", rows)
	}
}

func TestTheCapIsAskedAgainAfterTheBytesAreRead(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace"), "quote")
	files := e.files("mirror")
	files.Blob = changingStore{Store: e.blob, change: func() { e.opensOn(t, "mirror", "now()") }}
	if _, err := e.open(e.reader(t, principal.RowScopeAll), t, files, file); !errors.Is(err, extension.ErrAttachmentCapReached) {
		t.Errorf("the cap filled during the read: err = %v, want ErrAttachmentCapReached", err)
	}
	if rows := e.exports(t, Opened); rows != extension.MaxAttachmentOpensPerDay {
		t.Errorf("%d export rows, want only the %d written during the read", rows, extension.MaxAttachmentOpensPerDay)
	}
}

func TestAReadThatFailsLeavesNoExportRow(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	ctx := e.reader(t, principal.RowScopeAll)
	mail := e.mail(t, "workspace")
	larger, missing := e.file(t, mail, "larger"), e.file(t, mail, "missing")
	key := func(id ids.UUID) string {
		return e.WsScalar(t, `SELECT storage_key FROM attachment WHERE id = $1`, id)
	}
	past := strings.Repeat("x", extension.MaxInboundFileBytes+1)
	if err := e.blob.Put(context.Background(), key(larger), strings.NewReader(past), int64(len(past)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if err := e.blob.Delete(context.Background(), key(missing)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.open(ctx, t, e.files("mirror"), larger); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("an object larger than its recorded size: err = %v, want ErrInvalid", err)
	}
	if _, err := e.files("mirror").Open(ctx, missing); err == nil {
		t.Error("an object the store does not hold was opened")
	}
	if rows := e.exports(t, Opened); rows != 0 {
		t.Errorf("failed reads left %d export rows, want 0", rows)
	}
}

func TestOpensOfAnEarlierUTCDayDoNotCountTowardsTheCap(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace"), "quote")
	e.opensOn(t, "mirror", "now() - interval '1 day'")
	if _, err := e.open(e.reader(t, principal.RowScopeAll), t, e.files("mirror"), file); err != nil {
		t.Errorf("the first open of the day after a full day: err = %v, want it opened", err)
	}
}

func TestADealsMailLeavesOutAMailUnderARestriction(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	shown, restricted := e.mail(t, "workspace"), e.restrictedMail(t)
	query, err := ParseQuery(extension.MailQuery{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := ReadDeal(e.Admin(), e.Pool, e.SeedWonDealLinkedTo(t, shown, restricted), query)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != shown.String() {
		t.Errorf("the deal's mail = %+v, %v; want only %s, not the restricted %s", page.Items, err, shown, restricted)
	}
}
