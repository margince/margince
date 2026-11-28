// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchtext

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	vietnameseNote    = "Gửi anh xem.\n\n"
	vietnameseForward = vietnameseNote + "---------- Thư được chuyển tiếp ---------\nTừ: Nguyễn Văn An <an@kunde.vn>\nNgày: Th 3, 12 thg 8, 2026\nTiêu đề: Báo giá\nĐến: Sales\n\nGỗ teak giao thứ Ba."
	vietnameseReply   = "Đồng ý.\n\nTừ: Nguyễn Văn An <an@kunde.vn>\nĐã gửi: Thứ Ba, 12 tháng 8, 2026 09:14\nĐến: Sales\nChủ đề: Báo giá\n\nGỗ walnut được không?"
)

// storeEnd puts back the end an earlier release derived. The search trigger
// rebuilds the value from it, as that release stored it.
func storeEnd(t *testing.T, id ids.UUID, end string, args ...any) {
	t.Helper()
	owner := integration.FreshOwnerConn(t)
	if _, err := owner.Exec(context.Background(), `UPDATE activity SET body_own_end = `+end+` WHERE id = $1`, append([]any{id}, args...)...); err != nil {
		t.Fatalf("storing the earlier end: %v", err)
	}
}

func TestStoredMailIsReadAgainUnderTheVietnameseHeaderLabels(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	forward := logMail(t, e, "Fwd: Báo giá", vietnameseForward)
	reply := logMail(t, e, "Re: Báo giá", vietnameseReply)
	english := logMail(t, e, "Re: Cabinets", ownWords+quotedReply)
	storeEnd(t, forward, `char_length($2)`, vietnameseNote)
	storeEnd(t, reply, `char_length(body)`)
	storeEnd(t, english, `char_length(body)`)
	assertFound(t, e, forward, "teak", false)
	assertFound(t, e, reply, "walnut", true)
	const editMarks = `SELECT string_agg(id::text || ':' || version::text || ':' || updated_at::text, ',' ORDER BY id) FROM activity`
	marks := e.WsScalar(t, editMarks)

	task := activities.OwnEndRereadDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	e.RunDataTask(ctx, t, task)

	assertFound(t, e, forward, "teak", true)
	assertFound(t, e, reply, "walnut", false)
	// Only a body holding a Vietnamese label is read again.
	assertFound(t, e, english, "walnut", true)
	if got := e.WsCount(t, outOfStep); got != 0 {
		t.Errorf("%d rows hold a search value their text does not produce", got)
	}
	if got := e.WsScalar(t, editMarks); got != marks {
		t.Error("the reread moved an edit version or an updated_at; no member edited these rows")
	}

	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := activities.RereadOwnEndTx(ctx, tx, datatask.BatchIn{Limit: 100})
		if err != nil {
			return err
		}
		if out.Changed != 0 {
			t.Errorf("a replay wrote %d rows that were already right", out.Changed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
